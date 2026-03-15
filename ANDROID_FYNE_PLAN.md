# Go-Whisper Android App (Fyne) Design & Implementation Plan

## 1. Project Overview
This plan outlines how to port the Go-based `zop` CLI (with whisper.cpp support)
to an Android application using the Fyne toolkit. The intent is to reuse existing
provider, configuration, and chat session logic while introducing a mobile-ready
UI layer and audio pipeline.

## 2. System Architecture
The Android app will use an all-Go stack to reduce context switching between
languages and simplify maintenance.

- **GUI Framework**: Fyne (v2.x)
- **Audio Capture**: `malgo` (recommended for microphone input because it
  provides cross-platform capture callbacks); reserve `oto` for playback if
  needed, since it is primarily output-focused.
- **Inference Engine**: whisper.cpp via CGO bindings (existing `internal/whisper`)
- **Cross-Compilation Tool**: `fyne-cross` (Docker-based)

## 3. UI/UX Design
The interface is a single-page reactive layout.

### Layout Mockup
- **Top Bar**
  - Status indicator label ("Idle", "Listening", "Processing")
  - Configuration button (opens a dedicated settings window)
- **Center**
  - Scrollable read-only `widget.Entry` (disabled) for transcriptions/chat to
    enable selection/copy; fall back to a wrapped `widget.Label` if scrolling
    slows with very long transcripts (e.g., 1k+ lines) or Android memory
    pressure is observed.
- **Center-bottom**
  - Text entry box for user prompts
- **Bottom Bar**
  - Record/Stop button (primary action)
  - Clear button (icon-only)
  - Copy button (clipboard)
  - Agent selector button

## 4. Implementation Roadmap

### Phase 1: Environment Preparation
1. Install `fyne-cross`:
   ```sh
   go install github.com/fyne-io/fyne-cross@latest
   ```
2. Ensure Docker is running so `fyne-cross` can pull Android NDK images.
3. Validate Fyne works locally with a minimal sample app.

### Phase 2: App Scaffolding & Core Logic
1. Initialize a Fyne app entrypoint under `cmd/zop-mobile` (consistent with the
   existing `cmd/zop` layout). Use `internal/app` for shared UI helpers if needed.
2. Refactor CLI entry points to reuse a shared controller layer:
   - Prompt execution logic from `internal/cli`
   - Provider invocation (`internal/provider`)
   - Config handling (`internal/config`)
   - Chat session storage (`internal/chat`)
3. Build UI state management around a background controller goroutine.

### Phase 3: Asset Management (Whisper Model Files)
1. On first boot, check `os.UserConfigDir()` for the required `.bin` model.
2. If absent, download it into the app's config directory.
3. Keep the model path in config (or via `ZOP_WHISPER_MODEL` override).

### Phase 4: Hardware Integration
1. Update Fyne metadata to include `android.permission.RECORD_AUDIO`.
2. Implement audio capture at 16kHz mono PCM and stream into whisper buffers.
3. Display a pre-permission dialog describing why mic access is required.

### Phase 5: Build & Deploy
Build the APK targeting ARM64:
```sh
fyne-cross android -app-id com.zop.app -icon icon.png
```

## 5. Technical Constraints & Solutions
- **CGO Performance**: Call `runtime.LockOSThread()` for the inference goroutine.
- **Memory Management**: Prefer "Tiny" or "Base" quantized models.
- **Permissions UX**: Provide a user-facing dialog before system permission prompt.

## 6. Development Checklist
- [ ] Initialize the Fyne app package and verify a basic window renders.
- [ ] Create a shared controller layer to reuse CLI/provider logic.
- [ ] Implement the main UI with `container.NewBorder`.
- [ ] Add configuration window and agent selector UI.
- [ ] Wire prompt entry to provider execution (streamed responses).
- [ ] Add whisper model download/caching on first launch.
- [ ] Integrate microphone capture pipeline (16kHz mono PCM).
- [ ] Add RECORD_AUDIO permission and pre-permission dialog.
- [ ] Hook up Record/Stop, Clear, Copy actions.
- [ ] Build APK with `fyne-cross` and test on physical hardware.
