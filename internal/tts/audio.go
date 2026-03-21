//go:build tts

package tts

import (
	"sync"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
)

type audioPlayer struct {
	mctx          *malgo.AllocatedContext
	device        *malgo.Device
	mu            sync.Mutex
	queue         [][]float32
	samplesToWait int64
	sampleRate    int
}

func newAudioPlayer(sampleRate int) (*audioPlayer, error) {
	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}

	p := &audioPlayer{
		mctx:       mctx,
		sampleRate: sampleRate,
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatF32
	cfg.Playback.Channels = 1
	cfg.SampleRate = uint32(sampleRate)
	cfg.Alsa.NoMMap = 1

	device, err := malgo.InitDevice(mctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(pOutput, pInput []byte, frameCount uint32) {
			p.onAudio(pOutput, pInput, frameCount)
		},
	})
	if err != nil {
		_ = mctx.Uninit()
		mctx.Free()
		return nil, err
	}
	p.device = device

	if err := device.Start(); err != nil {
		device.Uninit()
		_ = mctx.Uninit()
		mctx.Free()
		return nil, err
	}

	return p, nil
}

func (p *audioPlayer) Play(samples []float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queue = append(p.queue, samples)
	p.samplesToWait += int64(len(samples))
}

func (p *audioPlayer) Wait() {
	// 1. Wait until queue is empty and all samples have been processed by the callback.
	start := time.Now()
	for {
		p.mu.Lock()
		busy := len(p.queue) > 0 || p.samplesToWait > 0
		p.mu.Unlock()
		if !busy {
			break
		}
		if time.Since(start) > 100*time.Second {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 2. Hardware Buffer Drain
	// Even after the callback processes the last sample, there is still audio in the 
	// hardware buffer. We MUST wait for this to finish to avoid the mic catching it.
	time.Sleep(100 * time.Millisecond)
}

func (p *audioPlayer) onAudio(pOutput, pInput []byte, frameCount uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// ALWAYS clear pOutput (silence)
	for i := range pOutput {
		pOutput[i] = 0
	}

	if len(p.queue) == 0 {
		return
	}

	totalBytesNeeded := uint32(len(pOutput))
	var bytesWritten uint32

	for bytesWritten < totalBytesNeeded && len(p.queue) > 0 {
		current := p.queue[0]
		bytesInCurrent := uint32(len(current) * 4)
		
		toCopy := totalBytesNeeded - bytesWritten
		if bytesInCurrent < toCopy {
			toCopy = bytesInCurrent
		}

		if toCopy > 0 {
			src := unsafe.Slice((*byte)(unsafe.Pointer(&current[0])), len(current)*4)
			copy(pOutput[bytesWritten:], src[:toCopy])
			
			bytesWritten += toCopy
			p.samplesToWait -= int64(toCopy / 4)

			if toCopy == bytesInCurrent {
				p.queue = p.queue[1:]
			} else {
				samplesConsumed := toCopy / 4
				p.queue[0] = current[samplesConsumed:]
			}
		} else {
			break
		}
	}
}

func (p *audioPlayer) Close() error {
	p.Wait()
	if p.device != nil {
		p.device.Uninit()
	}
	if p.mctx != nil {
		_ = p.mctx.Uninit()
		p.mctx.Free()
	}
	return nil
}
