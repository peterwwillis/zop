//go:build fyne || android

package mobileui

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/BurntSushi/toml"

	zopapp "github.com/peterwwillis/zop/internal/app"
	"github.com/peterwwillis/zop/internal/config"
	"github.com/peterwwillis/zop/internal/provider"
	"github.com/peterwwillis/zop/internal/whisper"
)

const (
	statusIdle       = "Idle"
	statusListening  = "Listening"
	statusProcessing = "Processing"
)

// NewWindow constructs the main Fyne window for the Android UI.
func NewWindow(app fyne.App, controller *zopapp.Controller) fyne.Window {
	window := app.NewWindow("zop")

	statusLabel := widget.NewLabelWithStyle(statusIdle, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	statusLabel.Wrapping = fyne.TextTruncate

	outputEntry := widget.NewMultiLineEntry()
	outputEntry.Wrapping = fyne.TextWrapWord
	outputEntry.Disable()

	outputScroll := container.NewVScroll(outputEntry)
	outputScroll.SetMinSize(fyne.NewSize(520, 320))

	buffer := newTranscriptBuffer(outputEntry, outputScroll)
	buffer.SetText(formatMessages(controller.Messages()))

	promptEntry := widget.NewEntry()
	promptEntry.SetPlaceHolder("Type a message and press Enter…")

	updateTitle := func() {
		window.SetTitle(fmt.Sprintf("zop (%s)", controller.ActiveAgent()))
	}
	updateTitle()

	setStatus := func(status string) {
		statusLabel.SetText(status)
	}

	appendWarning := func(warning string) {
		if warning == "" {
			return
		}
		buffer.AppendDirect(fmt.Sprintf("[zop] %s\n\n", warning))
	}

	voiceOutToggle := widget.NewCheck("TTS", func(b bool) {
		if b {
			// Try to initialize speaker if not already
			if err := controller.ReloadConfig(); err != nil {
				dialog.NewError(err, window).Show()
			}
		}
	})

	sendPrompt := func(prompt string) {
		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			return
		}
		promptEntry.Disable()
		setStatus(statusProcessing)
		buffer.AppendDirect(fmt.Sprintf("You: %s\nAssistant: ", prompt))
		appendWarning(controller.MissingAPIKeyWarning())

		go func() {
			var streamed bool
			ctx := context.Background()
			resp, sendErr := controller.SendPrompt(ctx, prompt, func(chunk string) {
				streamed = true
				buffer.Append(chunk)
			})
			fyne.Do(func() {
				if sendErr != nil {
					dialog.NewError(sendErr, window).Show()
				} else {
					if !streamed {
						buffer.AppendDirect(resp)
					}
					// Auto-speak if enabled
					if voiceOutToggle.Checked {
						go func() {
							_ = controller.WaitSpeaker()
							_ = controller.Speak(context.Background(), resp)
						}()
					}
				}
				buffer.AppendDirect("\n\n")
				promptEntry.Enable()
				promptEntry.SetText("")
				setStatus(statusIdle)
			})
		}()
	}

	promptEntry.OnSubmitted = sendPrompt

	configButton := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		showConfigWindow(app, window, controller, buffer, updateTitle)
	})

	agentButton := widget.NewButtonWithIcon("", theme.AccountIcon(), func() {
		showAgentDialog(window, controller, updateTitle, func() {
			buffer.SetText(formatMessages(controller.Messages()))
		})
	})

	copyButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		app.Clipboard().SetContent(buffer.Text())
		dialog.NewInformation("Copied", "Transcript copied to the clipboard.", window).Show()
	})

	clearButton := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		if err := controller.ClearSession(); err != nil {
			dialog.NewError(err, window).Show()
			return
		}
		buffer.Clear()
		setStatus(statusIdle)
	})

	recordButton := widget.NewButtonWithIcon("Record", theme.MediaRecordIcon(), nil)
	recording := false
	recordButton.OnTapped = func() {
		if recording {
			dialog.NewInformation("Recording", "Recording cannot be stopped while transcription is in progress. It will complete automatically once transcription finishes.", window).Show()
			return
		}
		dialog.NewConfirm("Microphone Access", "zop needs access to your microphone to transcribe audio.", func(ok bool) {
			if !ok {
				return
			}
			recording = true
			recordButton.SetText("Stop")
			recordButton.SetIcon(theme.MediaStopIcon())
			setStatus(statusListening)
			go func() {
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				text, rerr := whisper.RecordAndTranscribe()
				fyne.Do(func() {
					recording = false
					recordButton.SetText("Record")
					recordButton.SetIcon(theme.MediaRecordIcon())
					setStatus(statusIdle)
					if rerr != nil {
						dialog.NewError(rerr, window).Show()
						return
					}
					promptEntry.SetText(strings.TrimSpace(text))
					window.Canvas().Focus(promptEntry)
				})
			}()
		}, window).Show()
	}

	topBar := container.NewHBox(statusLabel, layout.NewSpacer(), voiceOutToggle, configButton)
	bottomBar := container.NewHBox(recordButton, layout.NewSpacer(), clearButton, copyButton, agentButton)
	center := container.NewBorder(nil, promptEntry, nil, nil, outputScroll)
	content := container.NewBorder(topBar, bottomBar, nil, nil, center)

	window.SetContent(content)
	window.Resize(fyne.NewSize(680, 540))
	return window
}

func formatMessages(messages []provider.Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		label := "Message"
		switch msg.Role {
		case "system":
			label = "System"
		case "user":
			label = "You"
		case "assistant":
			label = "Assistant"
		}
		if msg.Content == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n\n", label, msg.Content))
	}
	return sb.String()
}

type transcriptBuffer struct {
	mu     sync.Mutex
	text   strings.Builder
	entry  *widget.Entry
	scroll *container.Scroll
}

func newTranscriptBuffer(entry *widget.Entry, scroll *container.Scroll) *transcriptBuffer {
	return &transcriptBuffer{
		entry:  entry,
		scroll: scroll,
	}
}

func (b *transcriptBuffer) Append(value string) {
	current := b.appendLocked(value)
	fyne.Do(func() {
		b.applyText(current)
	})
}

func (b *transcriptBuffer) AppendDirect(value string) {
	current := b.appendLocked(value)
	b.applyText(current)
}

func (b *transcriptBuffer) Clear() {
	b.mu.Lock()
	b.text.Reset()
	b.mu.Unlock()
	b.applyText("")
	b.scroll.ScrollToTop()
}

func (b *transcriptBuffer) SetText(value string) {
	b.mu.Lock()
	b.text.Reset()
	b.text.WriteString(value)
	b.mu.Unlock()
	b.applyText(value)
}

func (b *transcriptBuffer) Text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.String()
}

func (b *transcriptBuffer) appendLocked(value string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text.WriteString(value)
	return b.text.String()
}

func (b *transcriptBuffer) applyText(value string) {
	b.entry.SetText(value)
	b.scroll.ScrollToBottom()
}

func showConfigWindow(app fyne.App, parent fyne.Window, controller *zopapp.Controller, buffer *transcriptBuffer, updateTitle func()) {
	cfgWindow := app.NewWindow("Configuration")
	path := controller.ConfigPath()
	pathLabel := widget.NewLabel(fmt.Sprintf("Config path: %s", path))
	pathLabel.Wrapping = fyne.TextWrapWord

	configEntry := widget.NewMultiLineEntry()
	configEntry.Wrapping = fyne.TextWrapWord

	loadConfig := func() {
		data, err := os.ReadFile(path)
		if err != nil {
			dialog.NewError(err, parent).Show()
			return
		}
		configEntry.SetText(string(data))
	}
	loadConfig()

	saveButton := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		if err := validateConfigText(configEntry.Text); err != nil {
			dialog.NewError(fmt.Errorf("invalid config: %w", err), parent).Show()
			return
		}
		if err := os.WriteFile(path, []byte(configEntry.Text), 0600); err != nil {
			dialog.NewError(err, parent).Show()
			return
		}
		if err := controller.ReloadConfig(); err != nil {
			dialog.NewError(err, parent).Show()
			return
		}
		updateTitle()
		buffer.SetText(formatMessages(controller.Messages()))
		dialog.NewInformation("Saved", "Configuration saved and reloaded successfully.", cfgWindow).Show()
	})
	reloadButton := widget.NewButtonWithIcon("Reload", theme.ViewRefreshIcon(), loadConfig)

	buttons := container.NewHBox(layout.NewSpacer(), reloadButton, saveButton)
	content := container.NewBorder(pathLabel, buttons, nil, nil, container.NewVScroll(configEntry))
	cfgWindow.SetContent(content)
	cfgWindow.Resize(fyne.NewSize(720, 520))
	cfgWindow.Show()
}

func showAgentDialog(parent fyne.Window, controller *zopapp.Controller, updateTitle func(), refreshTranscript func()) {
	agents := controller.AgentNames()
	if len(agents) == 0 {
		dialog.NewInformation("Agents", "No agents available in configuration.", parent).Show()
		return
	}

	selectWidget := widget.NewSelect(agents, nil)
	selectWidget.SetSelected(controller.ActiveAgent())

	items := []*widget.FormItem{
		widget.NewFormItem("Agent", selectWidget),
	}
	dialog.NewForm("Select Agent", "Apply", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}
		if err := controller.SetAgent(selectWidget.Selected); err != nil {
			dialog.NewError(err, parent).Show()
			return
		}
		updateTitle()
		refreshTranscript()
	}, parent).Show()
}

func validateConfigText(text string) error {
	var raw config.RawConfig
	_, err := toml.Decode(text, &raw)
	return err
}
