// Package cli implements the zop command-line interface.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/peterwwillis/zop/internal/chat"
	"github.com/peterwwillis/zop/internal/config"
	"github.com/peterwwillis/zop/internal/provider"
	"github.com/peterwwillis/zop/internal/whisper"
)

// Execute runs the root command with the provided arguments.
func Execute(args []string) {
	root := newRootCmd()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// globalFlags are shared across the root command and subcommands.
type globalFlags struct {
	configFile string
	agent      string
	verbose    bool
	debug      bool
}

func newRootCmd() *cobra.Command {
	gf := &globalFlags{}

	root := &cobra.Command{
		Use:   "zop [flags] [prompt]",
		Short: "zop – an AI CLI tool",
		Long: `zop is a multi-provider AI assistant for the command line.

Supported providers: openai, anthropic, google, openrouter, ollama.

The prompt can be supplied as:
  - Prompt flag:  zop -p "hello"
  - Command-line argument:  zop "hello"
  - Standard input (pipe):  echo "hello" | zop
  - Microphone (whisper-enabled builds):  zop --voice`,
		Example: `  zop "What is the capital of France?"
  echo "Explain recursion" | zop
  zop --agent claude "Review this code"
  zop --chat mysession "Continue our conversation"
  zop --interactive --chat mysession`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletion(cmd, args, gf)
		},
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
	}

	// Global flags
	root.PersistentFlags().StringVarP(&gf.configFile, "config", "C", "", "config file (default: ~/.config/zop/config.toml)")
	root.PersistentFlags().StringVarP(&gf.agent, "agent", "a", "default", "agent to use (defined in config)")
	root.PersistentFlags().BoolVarP(&gf.verbose, "verbose", "v", false, "verbose output")
	root.PersistentFlags().BoolVarP(&gf.debug, "debug", "d", false, "enable debug diagnostics (sets ZOP_DEBUG_VAD=1)")

	// Completion-specific flags (attached to root so they appear in help)
	root.Flags().StringP("chat", "c", "", "chat session name for multi-turn conversations")
	root.Flags().StringP("prompt", "p", "", "prompt to send (default: read from stdin)")
	root.Flags().StringP("system", "S", "", "system prompt override")
	root.Flags().BoolP("interactive", "i", false, "interactive chat session")
	root.Flags().BoolP("stream", "s", false, "stream response to stdout")
	root.Flags().BoolP("voice", "V", false, "record prompt from microphone (requires whisper-enabled build)")

	// Subcommands
	root.AddCommand(newChatCmd(gf))
	root.AddCommand(newConfigCmd(gf))
	root.AddCommand(newVersionCmd())

	return root
}

func runCompletion(cmd *cobra.Command, args []string, gf *globalFlags) error {
	// Load config
	cfg, err := config.Load(gf.configFile)
	if err != nil {
		return err
	}

	// Resolve agent + provider + model
	agent, err := cfg.GetAgent(gf.agent)
	if err != nil {
		return err
	}
	provCfg, err := cfg.GetProvider(agent.Provider)
	if err != nil {
		return err
	}
	modelCfg, err := cfg.GetModel(agent.Model)
	if err != nil {
		return err
	}

	// Build provider
	prov, err := provider.New(gf.agent, cfg)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	if gf.debug {
		if err := os.Setenv("ZOP_DEBUG_VAD", "1"); err != nil {
			return fmt.Errorf("enabling debug diagnostics: %w", err)
		}
	}

	if gf.verbose {
		fmt.Fprintf(errOut, "[zop] agent=%s provider=%s model=%s\n",
			gf.agent, agent.Provider, modelCfg.ModelID)
		if gf.debug {
			fmt.Fprintln(errOut, "[zop] debug diagnostics enabled (ZOP_DEBUG_VAD=1)")
		}
	}

	voice, _ := cmd.Flags().GetBool("voice")
	promptFlag, _ := cmd.Flags().GetString("prompt")
	interactive, _ := cmd.Flags().GetBool("interactive")

	if voice && promptFlag != "" {
		return fmt.Errorf("cannot use --voice with --prompt")
	}
	if voice && len(args) > 0 {
		return fmt.Errorf("cannot use --voice with positional prompt arguments")
	}
	if promptFlag != "" && len(args) > 0 {
		return fmt.Errorf("cannot combine --prompt with positional prompt arguments")
	}

	readVoicePrompt := func() (string, error) {
		var progressFn func(string)
		if gf.verbose {
			fmt.Fprintln(errOut, "[zop] sending voice input to Whisper for transcription")
			progressFn = func(msg string) {
				fmt.Fprintf(errOut, "[zop] %s\n", msg)
			}
		}
		voicePrompt, rerr := whisper.RecordAndTranscribeWithProgress(progressFn)
		if rerr != nil {
			return "", rerr
		}
		voicePrompt = strings.TrimSpace(voicePrompt)
		if gf.verbose {
			fmt.Fprintf(errOut, "[zop] Whisper transcription complete (%d chars)\n", len(voicePrompt))
		}
		return voicePrompt, nil
	}

	var initialPrompt string
	switch {
	case voice:
		voicePrompt, rerr := readVoicePrompt()
		if rerr != nil {
			return fmt.Errorf("voice input: %w", rerr)
		}
		initialPrompt = voicePrompt
	case promptFlag != "":
		initialPrompt = promptFlag
	case len(args) > 0:
		initialPrompt = strings.Join(args, " ")
	case !interactive:
		data, rerr := io.ReadAll(cmd.InOrStdin())
		if rerr != nil {
			return fmt.Errorf("reading stdin: %w", rerr)
		}
		initialPrompt = strings.TrimRight(string(data), "\n")
	}

	if !interactive && initialPrompt == "" {
		return fmt.Errorf("no prompt provided – pass -p, arguments, pipe via stdin, or use --voice")
	}

	// Build messages
	var messages []provider.Message

	// System prompt: flag > agent config > model config
	systemOverride, _ := cmd.Flags().GetString("system")
	switch {
	case systemOverride != "":
		messages = append(messages, provider.Message{Role: "system", Content: systemOverride})
	case agent.SystemPrompt != "":
		messages = append(messages, provider.Message{Role: "system", Content: agent.SystemPrompt})
	case modelCfg.SystemPrompt != "":
		messages = append(messages, provider.Message{Role: "system", Content: modelCfg.SystemPrompt})
	}

	// Chat session
	chatName, _ := cmd.Flags().GetString("chat")
	var sessionMgr *chat.Manager
	if chatName != "" {
		sessionMgr, err = chat.NewManager("")
		if err != nil {
			return err
		}
		history, herr := sessionMgr.Get(chatName)
		if herr != nil {
			return herr
		}
		messages = append(messages, history...)
	}

	// Streaming
	streamFlag, _ := cmd.Flags().GetBool("stream")

	var streamFn func(string)
	if streamFlag {
		streamFn = func(chunk string) {
			fmt.Fprint(out, chunk)
		}
	}

	// Warn (but don't hard-fail) when a provider expects an API key and none is set.
	// Providers like Ollama legitimately have no key requirement.
	if provCfg.APIKeyEnv != "" && provCfg.APIKey() == "" {
		fmt.Fprintf(errOut, "[zop] warning: environment variable %s is not set\n", provCfg.APIKeyEnv)
	}

	sendPrompt := func(prompt string) error {
		if prompt == "" {
			return nil
		}
		if gf.verbose {
			fmt.Fprintf(errOut, "[zop] sending text to AI (%d chars)\n", len(prompt))
		}
		messages = append(messages, provider.Message{Role: "user", Content: prompt})
		req := provider.CompletionRequest{
			Messages:   messages,
			Model:      modelCfg,
			Stream:     streamFlag,
			StreamFunc: streamFn,
		}
		resp, rerr := prov.Complete(context.Background(), req)
		if rerr != nil {
			return rerr
		}
		if !streamFlag {
			fmt.Fprintln(out, resp.Content)
		} else {
			fmt.Fprintln(out)
		}
		messages = append(messages, provider.Message{Role: "assistant", Content: resp.Content})
		if chatName != "" && sessionMgr != nil {
			if err := sessionMgr.Save(chatName, messages); err != nil {
				fmt.Fprintf(errOut, "[zop] warning: could not save session: %v\n", err)
			}
		}
		return nil
	}

	if initialPrompt != "" {
		if err := sendPrompt(initialPrompt); err != nil {
			return err
		}
	}

	if !interactive {
		return nil
	}

	if voice {
		for {
			voicePrompt, rerr := readVoicePrompt()
			if rerr != nil {
				return fmt.Errorf("voice input: %w", rerr)
			}
			if err := sendPrompt(voicePrompt); err != nil {
				return err
			}
		}
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	for {
		fmt.Fprint(errOut, "> ")
		line, rerr := reader.ReadString('\n')
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			return fmt.Errorf("reading prompt: %w", rerr)
		}
		line = strings.TrimSpace(line)
		if line != "" {
			if err := sendPrompt(line); err != nil {
				return err
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
	}

	return nil
}
