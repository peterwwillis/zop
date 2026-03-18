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
	captureSampleRate          = 16000
	captureChannels            = 1
	captureMaxDuration         = 30 * time.Second
	captureSilenceStopDuration = 1200 * time.Millisecond
	captureSpeechThreshold     = 700
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

	recordCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintln(os.Stderr, "Recording… press Ctrl-C or wait for silence.")
	pcm, err := captureAudioPCM(recordCtx, captureMaxDuration, captureSilenceStopDuration)
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
		mu             sync.Mutex
		samples        []int16
		lastSpeechAt   time.Time
		detectedSpeech bool
		stopOnce       sync.Once
		done           = make(chan struct{})
		startedAt      = time.Now()
	)

	stopCapture := func() {
		stopOnce.Do(func() {
			close(done)
		})
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, _ uint32) {
			frameSamples := bytesToInt16(input)
			if len(frameSamples) == 0 {
				return
			}

			now := time.Now()
			mu.Lock()
			samples = append(samples, frameSamples...)
			if hasSpeech(frameSamples, captureSpeechThreshold) {
				detectedSpeech = true
				lastSpeechAt = now
			}
			shouldStop := now.Sub(startedAt) >= maxDuration || (detectedSpeech && now.Sub(lastSpeechAt) >= silenceDuration)
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

	select {
	case <-ctx.Done():
	case <-done:
	}

	if device.IsStarted() {
		if err := device.Stop(); err != nil {
			return nil, err
		}
	}

	mu.Lock()
	recorded := append([]int16(nil), samples...)
	mu.Unlock()

	return int16ToPCMFloat(recorded), nil
}
