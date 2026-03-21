//go:build tts

package tts

/*
#cgo CFLAGS: -I${SRCDIR}/_lib/include
#cgo LDFLAGS: -L${SRCDIR}/_lib/build/lib -lsherpa-onnx-c-api -lsherpa-onnx-core -lsherpa-onnx-kaldifst-core -lsherpa-onnx-fst -lsherpa-onnx-fstfar -lkaldi-decoder-core -lssentencepiece_core -lkaldi-native-fbank-core -lpiper_phonemize -lespeak-ng -lucd -lonnxruntime -lstdc++ -lm
#cgo linux LDFLAGS: -fopenmp
#include <stdlib.h>
#include <string.h>
#include "c-api.h"
*/
import "C"

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"github.com/peterwwillis/zop/internal/config"
)

const (
	defaultModelURL = "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/vits-piper-en_US-amy-low.tar.bz2"
	modelName       = "vits-piper-en_US-amy-low"
)

type cgoSpeaker struct {
	tts    *C.SherpaOnnxOfflineTts
	player *audioPlayer
	speed  float32
	mu     sync.Mutex
}

func newSpeaker(cfg config.TTSConfig) (Speaker, error) {
	modelPath := defaultModelPath()
	
	// Override defaults if provided in config
	url := defaultModelURL
	if cfg.ModelURL != "" {
		url = cfg.ModelURL
	}
	name := modelName
	if cfg.ModelName != "" {
		name = cfg.ModelName
	}
	piperModel := "en_US-amy-low.onnx"
	if cfg.PiperModel != "" {
		piperModel = cfg.PiperModel
	}

	if err := ensureModel(modelPath, url, name); err != nil {
		return nil, fmt.Errorf("ensuring model: %w", err)
	}

	// Configuration for sherpa-onnx
	var cConfig C.SherpaOnnxOfflineTtsConfig
	C.memset(unsafe.Pointer(&cConfig), 0, C.sizeof_SherpaOnnxOfflineTtsConfig)

	modelDir := filepath.Join(modelPath, name)
	
	cModel := C.CString(filepath.Join(modelDir, piperModel))
	defer C.free(unsafe.Pointer(cModel))
	cTokens := C.CString(filepath.Join(modelDir, "tokens.txt"))
	defer C.free(unsafe.Pointer(cTokens))
	cDataDir := C.CString(filepath.Join(modelDir, "espeak-ng-data"))
	defer C.free(unsafe.Pointer(cDataDir))

	cConfig.model.vits.model = cModel
	cConfig.model.vits.tokens = cTokens
	cConfig.model.vits.data_dir = cDataDir
	cConfig.model.num_threads = 1
	cConfig.model.debug = 0
	
	cProvider := C.CString("cpu")
	defer C.free(unsafe.Pointer(cProvider))
	cConfig.model.provider = cProvider

	tts := C.SherpaOnnxCreateOfflineTts(&cConfig)
	if tts == nil {
		return nil, fmt.Errorf("failed to create sherpa-onnx TTS instance")
	}

	speed := float32(1.0)
	if cfg.Speed > 0 {
		speed = cfg.Speed
	}

	return &cgoSpeaker{
		tts:   tts,
		speed: speed,
	}, nil
}

func (s *cgoSpeaker) SetSpeed(speed float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if speed > 0 {
		s.speed = speed
	}
}

func (s *cgoSpeaker) Speak(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}

	s.mu.Lock()
	speed := s.speed
	s.mu.Unlock()

	if os.Getenv("ZOP_DEBUG_TTS") == "1" {
		fmt.Fprintf(os.Stderr, "[zop] tts: generating audio for %d chars at speed %.2f...\n", len(text), speed)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	audio := C.SherpaOnnxOfflineTtsGenerate(s.tts, cText, 0, C.float(speed))
	if audio == nil {
		return fmt.Errorf("failed to generate audio")
	}
	defer C.SherpaOnnxDestroyOfflineTtsGeneratedAudio(audio)

	n := int(audio.n)
	if n == 0 {
		return nil
	}

	if os.Getenv("ZOP_DEBUG_TTS") == "1" {
		fmt.Fprintf(os.Stderr, "[zop] tts: generated %d samples at %d Hz\n", n, int(audio.sample_rate))
	}

	// Convert C float array to Go slice
	samples := (*[1 << 30]float32)(unsafe.Pointer(audio.samples))[:n:n]
	
	// Copy samples to a new slice to be safe
	goSamples := make([]float32, n)
	copy(goSamples, samples)

	// Lazily initialize or replace audio player if sample rate changed
	sampleRate := int(audio.sample_rate)
	if s.player == nil || s.player.sampleRate != sampleRate {
		if s.player != nil {
			s.player.Close()
		}
		player, err := newAudioPlayer(sampleRate)
		if err != nil {
			return fmt.Errorf("failed to create audio player: %w", err)
		}
		s.player = player
	}

	s.player.Play(goSamples)

	return nil
}

func (s *cgoSpeaker) Wait() error {
	s.mu.Lock()
	player := s.player
	s.mu.Unlock()

	if player != nil {
		player.Wait()
	}
	return nil
}

func (s *cgoSpeaker) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.player != nil {
		s.player.Close()
	}
	if s.tts != nil {
		C.SherpaOnnxDestroyOfflineTts(s.tts)
		s.tts = nil
	}
	return nil
}

func defaultModelPath() string {
	if p := os.Getenv("ZOP_TTS_MODEL"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "zop-tts")
	}
	return filepath.Join(home, ".local", "share", "zop", "tts")
}

func ensureModel(path, url, name string) error {
	if _, err := os.Stat(filepath.Join(path, name)); err == nil {
		return nil // already present
	}

	fmt.Fprintf(os.Stderr, "[zop] TTS model not found at %q – downloading from %s …\n", path, url)

	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("creating model directory: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("downloading model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading model: HTTP %d from %s", resp.StatusCode, url)
	}

	if err := extractTarBz2(resp.Body, path); err != nil {
		return fmt.Errorf("extracting model: %w", err)
	}

	return nil
}

func extractTarBz2(r io.Reader, dest string) error {
	bzr := bzip2.NewReader(r)
	tr := tar.NewReader(bzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
