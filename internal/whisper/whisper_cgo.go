//go:build whisper

// Package whisper provides voice-to-text input using Whisper.
// This file is compiled only when the "whisper" build tag is set.
//
// Build with: go build -tags whisper
//
// This implementation uses the whisper.cpp C library via CGo.
// See https://github.com/ggerganov/whisper.cpp for installation instructions.
// The whisper model file path can be set with ZOP_WHISPER_MODEL (defaults to
// ~/.local/share/zop/whisper/ggml-base.en.bin).
// If the model file does not exist at that path, it is downloaded automatically
// from https://huggingface.co/ggerganov/whisper.cpp.
package whisper

// #cgo CFLAGS: -I${SRCDIR}/whisper.cpp/include -I${SRCDIR}/whisper.cpp/ggml/include
// #cgo LDFLAGS: -L${SRCDIR}/whisper.cpp/build/src -L${SRCDIR}/whisper.cpp/build/ggml/src -lwhisper -lggml -lggml-base -lggml-cpu -lm -lstdc++
// #cgo linux LDFLAGS: -fopenmp
// #include <stdlib.h>
// #include "whisper.h"
// static void whisper_log_callback_silent(enum ggml_log_level level, const char * text, void * user_data) {
//   (void) level;
//   (void) text;
//   (void) user_data;
// }
// static void whisper_log_set_silent(void) {
//   whisper_log_set(whisper_log_callback_silent, NULL);
// }
// static void whisper_log_set_default(void) {
//   whisper_log_set(NULL, NULL);
// }
import "C"

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// defaultModelURL is where the base English Whisper model is fetched when the
// local file is absent.
const defaultModelURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin"

// defaultModelPath returns the path where the Whisper model should live.
// Override with ZOP_WHISPER_MODEL.
func defaultModelPath() string {
	if p := os.Getenv("ZOP_WHISPER_MODEL"); p != "" {
		return p
	}
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "zop", "whisper", "ggml-base.en.bin")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "ggml-base.en.bin"
	}
	return filepath.Join(home, ".local", "share", "zop", "whisper", "ggml-base.en.bin")
}

// ensureModel makes sure the model file exists at path.  If it does not, it
// downloads it from defaultModelURL and saves it to path.
func ensureModel(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already present
	}

	fmt.Fprintf(os.Stderr, "[zop] Whisper model not found at %q – downloading from %s …\n", path, defaultModelURL)

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating model directory: %w", err)
	}

	// Write to a temp file first so a partial download is never used.
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }()

	resp, err := http.Get(defaultModelURL) //nolint:gosec // URL is a compile-time constant
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("downloading model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_ = f.Close()
		return fmt.Errorf("downloading model: HTTP %d from %s", resp.StatusCode, defaultModelURL)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing model: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing model file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("installing model: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[zop] Whisper model saved to %q\n", path)
	return nil
}

// configureWhisperNativeLogging controls whisper.cpp's native stderr logging.
// By default we silence it to keep CLI output clean. When ZOP_DEBUG_VAD=1 is
// set (e.g. via `zop --debug`), we restore whisper's default logger.
func configureWhisperNativeLogging() {
	if os.Getenv("ZOP_DEBUG_VAD") == "1" {
		C.whisper_log_set_default()
		return
	}
	C.whisper_log_set_silent()
}

const (
	captureSampleRate       = 16000
	captureChannels         = 1
	captureMaxDuration      = 30 * time.Second
	captureStopAfterSilence = 1200 * time.Millisecond

	// Adaptive VAD constants.
	//
	// We compute frame RMS with DC-offset removed and compare it to a running
	// ambient-noise RMS estimate using SNR (in dB). This makes detection portable
	// across microphones with very different gain levels.
	captureNoiseEWMAAlpha = 0.08
	captureNoiseWarmup    = 400 * time.Millisecond

	captureSpeechStartDB     = 5.0
	captureSpeechStopDB      = 2.0
	captureSpeechStartMin    = 120 * time.Millisecond
	captureSpeechEndHangover = 220 * time.Millisecond

	// Floors to avoid divide-by-zero/near-zero instability.
	captureNoiseFloorRMS = 1.0
	captureMinVoiceRMS   = 120.0
)

// RecordAndTranscribe records audio from the default microphone and transcribes
// it using the local Whisper model. If the model file does not exist it is
// downloaded first.
func RecordAndTranscribe() (string, error) {
	return recordAndTranscribe(nil, true)
}

// RecordAndTranscribeWithProgress records audio and transcribes it with Whisper.
// If progress is non-nil, it receives lifecycle messages suitable for verbose
// stderr logging by callers.
func RecordAndTranscribeWithProgress(progress func(string)) (string, error) {
	return recordAndTranscribe(progress, true)
}

// RecordAndTranscribeManualWithProgress records audio and transcribes it with
// Whisper, but disables silence auto-stop. Recording ends when Ctrl-D is sent.
func RecordAndTranscribeManualWithProgress(progress func(string)) (string, error) {
	return recordAndTranscribe(progress, false)
}

func recordAndTranscribe(progress func(string), stopOnSilence bool) (string, error) {
	modelPath := defaultModelPath()

	if err := ensureModel(modelPath); err != nil {
		return "", fmt.Errorf("whisper model setup: %w", err)
	}
	configureWhisperNativeLogging()

	cModel := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cModel))

	params := C.whisper_context_default_params()
	ctx := C.whisper_init_from_file_with_params(cModel, params)
	if ctx == nil {
		return "", fmt.Errorf("failed to initialize whisper context from %q", modelPath)
	}
	defer C.whisper_free(ctx)

	recordCtx, stopRecording := context.WithCancel(context.Background())
	defer stopRecording()

	// Handle SIGTERM so the OS can cleanly stop a recording in progress.
	sigCtx, sigStop := signal.NotifyContext(recordCtx, syscall.SIGTERM)
	defer sigStop()

	// Watch for Ctrl+D (EOF on stdin) to stop recording without exiting.
	go func() {
		b := make([]byte, 1)
		for {
			if _, err := os.Stdin.Read(b); err != nil {
				stopRecording()
				return
			}
		}
	}()

	if stopOnSilence {
		fmt.Fprintln(os.Stderr, "Recording… calibrating noise floor briefly, then speak (Ctrl-D to stop).")
	} else {
		fmt.Fprintln(os.Stderr, "Recording… silence auto-stop disabled; press Ctrl-D when ready.")
	}
	pcm, err := captureAudioPCM(sigCtx, captureMaxDuration, captureStopAfterSilence, stopOnSilence)
	if err != nil {
		return "", fmt.Errorf("capturing audio: %w", err)
	}
	if len(pcm) == 0 {
		return "", fmt.Errorf("no audio captured")
	}
	if progress != nil {
		progress("recording stopped; Whisper transcription started")
	}

	wparams := C.whisper_full_default_params(C.WHISPER_SAMPLING_GREEDY)
	wparams.print_progress = false
	wparams.print_realtime = false
	wparams.print_timestamps = false
	wparams.translate = false
	wparams.no_context = true
	wparams.single_segment = false

	if rc := C.whisper_full(ctx, wparams, (*C.float)(unsafe.Pointer(&pcm[0])), C.int(len(pcm))); rc != 0 {
		return "", fmt.Errorf("whisper inference failed (code %d)", int(rc))
	}

	var out strings.Builder
	segments := int(C.whisper_full_n_segments(ctx))
	for i := 0; i < segments; i++ {
		seg := strings.TrimSpace(C.GoString(C.whisper_full_get_segment_text(ctx, C.int(i))))
		if seg == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteByte(' ')
		}
		out.WriteString(seg)
	}

	text := strings.TrimSpace(out.String())
	if text == "" {
		return "", fmt.Errorf("no speech recognized")
	}
	return text, nil
}

// bytesToFloat32View creates a zero-allocation float32 view over a byte slice.
// The returned slice is only valid as long as the underlying byte slice is valid.
// It is safe to use within the callback as we copy the data out before returning.
func bytesToFloat32View(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	return unsafe.Slice((*float32)(unsafe.Pointer(&b[0])), len(b)/4)
}

func captureAudioPCM(ctx context.Context, maxDuration, silenceDuration time.Duration, stopOnSilence bool) ([]float32, error) {
	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = mctx.Uninit()
		mctx.Free()
	}()

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = captureChannels
	cfg.SampleRate = captureSampleRate

	durationToSamples := func(d time.Duration) int64 {
		s := int64(captureSampleRate) * int64(d) / int64(time.Second)
		if s < 1 {
			return 1
		}
		return s
	}
	vadDebug := os.Getenv("ZOP_DEBUG_VAD") == "1"

	var (
		mu               sync.Mutex
		samples          []float32
		detectedSpeech   bool
		speechActive     bool
		totalSamples     int64
		lastSpeechSample int64
		speechAboveStart int64
		speechBelowStop  int64
		stopOnce         sync.Once
		done             = make(chan struct{})

		noiseRMS         float64
		noiseInitialized bool
		lastDebugSample  int64
	)
	speechStartSamples := durationToSamples(captureSpeechStartMin)
	speechEndSamples := durationToSamples(captureSpeechEndHangover)
	noiseWarmupSamples := durationToSamples(captureNoiseWarmup)
	debugEverySamples := durationToSamples(250 * time.Millisecond)

	stopCapture := func() {
		stopOnce.Do(func() {
			close(done)
		})
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, _ uint32) {
			frameSamples := bytesToFloat32View(input)
			if len(frameSamples) == 0 {
				return
			}

			mu.Lock()
			samples = append(samples, frameSamples...)

			frameCount := int64(len(frameSamples)) / int64(captureChannels)
			if frameCount <= 0 {
				mu.Unlock()
				return
			}
			totalSamples += frameCount

			// Keep VAD RMS values in int16-equivalent units for readable logs.
			// We remove DC offset first so microphones with constant bias don't look
			// like continuous speech.
			frameRMS := rmsAmplitudeFloat32(frameSamples) * 32768.0

			if !noiseInitialized {
				noiseRMS = math.Max(frameRMS, captureNoiseFloorRMS)
				noiseInitialized = true
			}
			noiseRMS = math.Max(noiseRMS, captureNoiseFloorRMS)

			inWarmup := totalSamples <= noiseWarmupSamples

			if inWarmup {
				// During warmup, only estimate ambient noise and suppress speech state.
				noiseRMS += captureNoiseEWMAAlpha * (frameRMS - noiseRMS)
				noiseRMS = math.Max(noiseRMS, captureNoiseFloorRMS)
				speechAboveStart = 0
				speechBelowStop = 0
				speechActive = false
			}

			snrDB := 20.0 * math.Log10((frameRMS+captureNoiseFloorRMS)/(noiseRMS+captureNoiseFloorRMS))

			if !inWarmup {
				if !speechActive {
					if frameRMS >= captureMinVoiceRMS && snrDB >= captureSpeechStartDB {
						speechAboveStart += frameCount
						if speechAboveStart >= speechStartSamples {
							speechActive = true
							detectedSpeech = true
							lastSpeechSample = totalSamples
							speechAboveStart = 0
							speechBelowStop = 0
						}
					} else {
						speechAboveStart = 0
						// Update ambient noise estimate only while not in speech.
						noiseRMS += captureNoiseEWMAAlpha * (frameRMS - noiseRMS)
						noiseRMS = math.Max(noiseRMS, captureNoiseFloorRMS)
					}
				} else {
					if frameRMS >= captureMinVoiceRMS && snrDB >= captureSpeechStopDB {
						speechBelowStop = 0
						lastSpeechSample = totalSamples
					} else {
						speechBelowStop += frameCount
						if speechBelowStop >= speechEndSamples {
							speechActive = false
							speechBelowStop = 0
						}
					}
				}
			}

			sampleRate := int64(captureSampleRate)
			elapsedSinceStart := time.Duration(totalSamples) * time.Second / time.Duration(sampleRate)

			var elapsedSinceLastSpeech time.Duration
			if detectedSpeech {
				silenceSamples := totalSamples - lastSpeechSample
				if silenceSamples < 0 {
					silenceSamples = 0
				}
				elapsedSinceLastSpeech = time.Duration(silenceSamples) * time.Second / time.Duration(sampleRate)
			}

			if vadDebug && totalSamples-lastDebugSample >= debugEverySamples {
				fmt.Fprintf(
					os.Stderr,
					"[zop] vad rms=%.1f noise=%.1f snr_db=%.2f start_db=%.2f stop_db=%.2f warmup=%t active=%t detected=%t silence_ms=%d\n",
					frameRMS,
					noiseRMS,
					snrDB,
					captureSpeechStartDB,
					captureSpeechStopDB,
					inWarmup,
					speechActive,
					detectedSpeech,
					elapsedSinceLastSpeech/time.Millisecond,
				)
				lastDebugSample = totalSamples
			}

			reachedMaxDuration := elapsedSinceStart >= maxDuration
			reachedSilenceStop := stopOnSilence && detectedSpeech && !speechActive && elapsedSinceLastSpeech >= silenceDuration
			shouldStop := reachedMaxDuration || reachedSilenceStop
			mu.Unlock()

			if shouldStop {
				stopCapture()
			}
		},
	}

	device, err := malgo.InitDevice(mctx.Context, cfg, deviceCallbacks)
	if err != nil {
		return nil, err
	}
	defer device.Uninit()

	if err := device.Start(); err != nil {
		return nil, err
	}

	// Enforce maxDuration even if no audio callbacks are delivered by using a
	// context with timeout for the blocking wait below. This ensures we don't
	// hang indefinitely when the device is started but never produces data.
	waitCtx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()

	select {
	case <-waitCtx.Done():
	case <-done:
	}

	if device.IsStarted() {
		_ = device.Stop()
	}

	// Return whatever was recorded regardless of why we stopped (silence
	// detection, max-duration timeout, or Ctrl-C / SIGTERM). The caller
	// checks for empty audio and surfaces a clear error in that case.

	mu.Lock()
	recorded := append([]float32(nil), samples...)
	mu.Unlock()

	return recorded, nil
}
