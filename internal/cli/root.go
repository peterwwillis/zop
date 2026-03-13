// Package cli implements the pgpt command-line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/peterwwillis/pgpt/internal/chat"
	"github.com/peterwwillis/pgpt/internal/config"
	"github.com/peterwwillis/pgpt/internal/provider"
	"github.com/peterwwillis/pgpt/internal/whisper"
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
}

func newRootCmd() *cobra.Command {
	gf := &globalFlags{}

	root := &cobra.Command{
		Use:   "pgpt [flags] [prompt]",
		Short: "PowerGPT – an AI CLI tool",
		Long: `pgpt (PowerGPT) is a multi-provider AI assistant for the command line.

Supported providers: openai, anthropic, google, openrouter, ollama.

The prompt can be supplied as:
  - Command-line argument:  pgpt "hello"
  - Standard input (pipe):  echo "hello" | pgpt
  - Microphone (if compiled with -tags whisper):  pgpt --voice`,
		Example: `  pgpt "What is the capital of France?"
  echo "Explain recursion" | pgpt
  pgpt --agent claude "Review this code"
  pgpt --chat mysession "Continue our conversation"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletion(cmd, args, gf)
		},
		SilenceUsage: true,
	}

	// Global flags
	root.PersistentFlags().StringVarP(&gf.configFile, "config", "C", "", "config file (default: ~/.config/pgpt/config.toml)")
	root.PersistentFlags().StringVarP(&gf.agent, "agent", "a", "default", "agent to use (defined in config)")
	root.PersistentFlags().BoolVarP(&gf.verbose, "verbose", "v", false, "verbose output")

	// Completion-specific flags (attached to root so they appear in help)
	root.Flags().StringP("chat", "c", "", "chat session name for multi-turn conversations")
	root.Flags().StringP("system", "S", "", "system prompt override")
	root.Flags().BoolP("stream", "s", false, "stream response to stdout")
	root.Flags().BoolP("voice", "V", false, "record prompt from microphone (requires -tags whisper)")

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

	if gf.verbose {
		fmt.Fprintf(os.Stderr, "[pgpt] agent=%s provider=%s model=%s\n",
			gf.agent, agent.Provider, modelCfg.ModelID)
	}

	// Build prompt
	voice, _ := cmd.Flags().GetBool("voice")
	var prompt string

	switch {
	case voice:
		prompt, err = whisper.RecordAndTranscribe()
		if err != nil {
			return fmt.Errorf("voice input: %w", err)
		}
	case len(args) > 0:
		prompt = strings.Join(args, " ")
	default:
		// Try reading from stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, rerr := io.ReadAll(os.Stdin)
			if rerr != nil {
				return fmt.Errorf("reading stdin: %w", rerr)
			}
			prompt = strings.TrimRight(string(data), "\n")
		}
	}

	if prompt == "" {
		return fmt.Errorf("no prompt provided – pass as argument, pipe via stdin, or use --voice")
	}

	// Build messages
	var messages []provider.Message

	// System prompt: flag > agent config > nothing
	systemOverride, _ := cmd.Flags().GetString("system")
	switch {
	case systemOverride != "":
		messages = append(messages, provider.Message{Role: "system", Content: systemOverride})
	case agent.SystemPrompt != "":
		messages = append(messages, provider.Message{Role: "system", Content: agent.SystemPrompt})
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

	messages = append(messages, provider.Message{Role: "user", Content: prompt})

	// Streaming
	streamFlag, _ := cmd.Flags().GetBool("stream")

	var streamFn func(string)
	if streamFlag {
		streamFn = func(chunk string) {
			fmt.Print(chunk)
		}
	}

	// Warn (but don't hard-fail) when a provider expects an API key and none is set.
	// Providers like Ollama legitimately have no key requirement.
	if provCfg.APIKeyEnv != "" && provCfg.APIKey() == "" {
		fmt.Fprintf(os.Stderr, "[pgpt] warning: environment variable %s is not set\n", provCfg.APIKeyEnv)
	}

	req := provider.CompletionRequest{
		Messages:   messages,
		Model:      modelCfg,
		Stream:     streamFlag,
		StreamFunc: streamFn,
	}

	resp, err := prov.Complete(context.Background(), req)
	if err != nil {
		return err
	}

	// Print response (streaming already printed above)
	if !streamFlag {
		fmt.Println(resp.Content)
	} else {
		fmt.Println() // newline after streamed output
	}

	// Persist chat session
	if chatName != "" && sessionMgr != nil {
		messages = append(messages, provider.Message{Role: "assistant", Content: resp.Content})
		if err := sessionMgr.Save(chatName, messages); err != nil {
			fmt.Fprintf(os.Stderr, "[pgpt] warning: could not save session: %v\n", err)
		}
	}

	return nil
}
