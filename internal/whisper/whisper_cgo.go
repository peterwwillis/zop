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
import "C"

import (
	"context"
	"fmt"
	"io"
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

const (
	captureSampleRate       = 16000
	captureChannels         = 1
	captureMaxDuration      = 30 * time.Second
	captureStopAfterSilence = 1200 * time.Millisecond
	// captureSpeechThreshold is an RMS amplitude threshold (int16 units).
	// Background noise in a quiet room is typically 100–300 RMS; conversational
	// speech is 1 000–5 000+. 800 sits comfortably between the two, so ambient
	// hiss or HVAC hum won't prevent the silence timer from advancing.
	captureSpeechThreshold = 800
)

// RecordAndTranscribe records audio from the default microphone and transcribes
// it using the local Whisper model. If the model file does not exist it is
// downloaded first.
func RecordAndTranscribe() (string, error) {
	modelPath := defaultModelPath()

	if err := ensureModel(modelPath); err != nil {
		return "", fmt.Errorf("whisper model setup: %w", err)
	}

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

	fmt.Fprintln(os.Stderr, "Recording… press Ctrl-D to stop, or wait for silence to stop automatically.")
	pcm, err := captureAudioPCM(sigCtx, captureMaxDuration, captureStopAfterSilence)
	if err != nil {
		return "", fmt.Errorf("capturing audio: %w", err)
	}
	if len(pcm) == 0 {
		return "", fmt.Errorf("no audio captured")
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

// bytesToInt16View creates a zero-allocation int16 view over a byte slice.
// The returned slice is only valid as long as the underlying byte slice is valid.
// It is safe to use within the callback as we copy the data out before returning.
func bytesToInt16View(b []byte) []int16 {
	if len(b) == 0 {
		return nil
	}
	return unsafe.Slice((*int16)(unsafe.Pointer(&b[0])), len(b)/2)
}

func captureAudioPCM(ctx context.Context, maxDuration, silenceDuration time.Duration) ([]float32, error) {
	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = mctx.Uninit()
		mctx.Free()
	}()

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = captureChannels
	cfg.SampleRate = captureSampleRate

	var (
		mu              sync.Mutex
		samples         []int16
		detectedSpeech  bool
		totalSamples    int64 // total samples captured across all channels
		lastSpeechSample int64
		stopOnce        sync.Once
		done            = make(chan struct{})
	)

	stopCapture := func() {
		stopOnce.Do(func() {
			close(done)
		})
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, _ uint32) {
			frameSamples := bytesToInt16View(input)
			if len(frameSamples) == 0 {
				return
			}

			mu.Lock()
			samples = append(samples, frameSamples...)

			// Number of frames in this callback (per channel).
			frameCount := int64(len(frameSamples)) / int64(captureChannels)
			if frameCount <= 0 {
				mu.Unlock()
				return
			}

			// Update sample counters.
			totalSamplesBefore := totalSamples
			totalSamples += frameCount

			if hasSpeech(frameSamples, captureSpeechThreshold) {
				detectedSpeech = true
				lastSpeechSample = totalSamples
			}

			// Convert sample counts to durations using the known sample rate.
			sampleRate := int64(captureSampleRate)
			elapsedSinceStart := time.Duration(totalSamples) * time.Second / time.Duration(sampleRate)

			var elapsedSinceLastSpeech time.Duration
			if detectedSpeech {
				silenceSamples := totalSamples - lastSpeechSample
				if silenceSamples < 0 {
					// Should not happen, but guard against negative durations.
					silenceSamples = 0
				}
				elapsedSinceLastSpeech = time.Duration(silenceSamples) * time.Second / time.Duration(sampleRate)
			}

			reachedMaxDuration := elapsedSinceStart >= maxDuration
			reachedSilenceStop := detectedSpeech && elapsedSinceLastSpeech >= silenceDuration
			_ = totalSamplesBefore // kept to preserve potential future use without changing semantics
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
	recorded := append([]int16(nil), samples...)
	mu.Unlock()

	return int16ToPCMFloat(recorded), nil
}
