// Package cli implements the zop command-line interface.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/peterwwillis/zop/internal/chat"
	"github.com/peterwwillis/zop/internal/config"
	"github.com/peterwwillis/zop/internal/mcp"
	"github.com/peterwwillis/zop/internal/provider"
	"github.com/peterwwillis/zop/internal/tool"
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
	noTools    bool
}

var sessionPartSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

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
	root.PersistentFlags().BoolVarP(&gf.noTools, "no-tools", "T", false, "disable tool calling support")

	// Completion-specific flags (attached to root so they appear in help)
	root.Flags().StringP("chat", "c", "", "chat session name for multi-turn conversations")
	root.Flags().StringP("prompt", "p", "", "prompt to send (default: read from stdin)")
	root.Flags().StringP("system", "S", "", "system prompt override")
	root.Flags().BoolP("interactive", "i", false, "interactive chat session")
	root.Flags().BoolP("stream", "s", false, "stream response to stdout")
	root.Flags().BoolP("voice", "V", false, "record prompt from microphone (requires whisper-enabled build)")
	root.Flags().Bool("voice-manual", false, "disable silence auto-stop in voice mode; press Ctrl-D when ready")

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

	// Initialize tools/MCP
	registry := tool.NewRegistry()
	policy := cfg.ToolPolicy
	if agent.ToolPolicy != nil {
		policy = *agent.ToolPolicy
	}
	registry.Register(&tool.RunCommandTool{
		Policy: tool.NewPolicyChecker(policy),
	})
	checker := tool.NewPolicyChecker(policy)
	for name, mcpCfg := range cfg.MCPServers {
		mcpClient, err := mcp.NewClient(context.Background(), mcpCfg.URL, mcpCfg.Command, mcpCfg.Args...)
		if err != nil {
			if gf.verbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "[zop] warning: failed to connect to MCP server %q: %v\n", name, err)
			}
			continue
		}
		wrappers, err := mcp.WrapTools(context.Background(), mcpClient, checker)
		if err != nil {
			if gf.verbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "[zop] warning: failed to list tools for MCP server %q: %v\n", name, err)
			}
			continue
		}
		for _, w := range wrappers {
			registry.Register(w)
		}
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
	voiceManual, _ := cmd.Flags().GetBool("voice-manual")
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
	if voiceManual && !voice {
		return fmt.Errorf("cannot use --voice-manual without --voice")
	}
	if gf.verbose && voice && voiceManual {
		fmt.Fprintln(errOut, "[zop] voice mode: manual stop (Ctrl-D)")
	}

	readVoicePrompt := func() (string, error) {
		var progressFn func(string)
		if gf.verbose {
			fmt.Fprintln(errOut, "[zop] sending voice input to Whisper for transcription")
			progressFn = func(msg string) {
				fmt.Fprintf(errOut, "[zop] %s\n", msg)
			}
		}
		var (
			voicePrompt string
			rerr        error
		)
		if voiceManual {
			voicePrompt, rerr = whisper.RecordAndTranscribeManualWithProgress(progressFn)
		} else {
			voicePrompt, rerr = whisper.RecordAndTranscribeWithProgress(progressFn)
		}
		if rerr != nil {
			return "", rerr
		}
		voicePrompt = strings.TrimSpace(voicePrompt)
		if gf.verbose {
			fmt.Fprintf(errOut, "[zop] Whisper transcription complete (%d chars)\n", len(voicePrompt))
			fmt.Fprintf(errOut, "[zop] transcription: %s\n", voicePrompt)
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

	// Load ZOP.md instructions
	zopInstructions, err := config.LoadZopInstructions(gf.configFile)
	if err != nil {
		if gf.verbose {
			fmt.Fprintf(errOut, "[zop] warning: could not load ZOP.md: %v\n", err)
		}
	} else if zopInstructions != "" {
		messages = append(messages, provider.Message{Role: "system", Content: zopInstructions})
	}

	useTools := !gf.noTools && !agent.DisableTools && !cfg.DisableTools
	if useTools && len(policy.AllowList) == 0 {
		if gf.verbose {
			fmt.Fprintln(errOut, "[zop] tools enabled but allow_list is empty; disabling tools for this request")
		}
		useTools = false
	}

	if useTools {
		messages = append(messages, provider.Message{
			Role:    "system",
			Content: "You have access to tools. Use them ONLY when explicitly required by the user's request or necessary to fulfill it. If you can provide a high-quality response without tools, do so.",
		})
	}

	// Keep the system-prompt slice so we can reset history when rotating to a
	// fresh session after hitting provider context limits.
	baseMessages := append([]provider.Message(nil), messages...)

	// Chat session
	chatName, _ := cmd.Flags().GetString("chat")
	autoChat := false
	var sessionMgr *chat.Manager
	if chatName != "" || interactive {
		sessionMgr, err = chat.NewManager("")
		if err != nil {
			return err
		}
	}

	if interactive && chatName == "" {
		autoChat = true
		lastAuto, err := sessionMgr.GetLastAutoSession(gf.agent)
		if err != nil {
			return err
		}
		if lastAuto != "" {
			exists, err := sessionMgr.Exists(lastAuto)
			if err != nil {
				return err
			}
			if exists {
				chatName = lastAuto
				if gf.verbose {
					fmt.Fprintf(errOut, "[zop] resumed automatic session %q\n", chatName)
				}
			} else if err := sessionMgr.SetLastAutoSession(gf.agent, ""); err != nil {
				return err
			}
		}
		if chatName == "" {
			chatName, err = nextUniqueSessionName(sessionMgr, "auto-"+sanitizeSessionNamePart(gf.agent))
			if err != nil {
				return err
			}
			if err := sessionMgr.SetLastAutoSession(gf.agent, chatName); err != nil {
				return err
			}
			if gf.verbose {
				fmt.Fprintf(errOut, "[zop] started automatic session %q\n", chatName)
			}
		}
	}

	if chatName != "" && sessionMgr != nil {
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

	rolloverSession := func() error {
		if sessionMgr == nil {
			return fmt.Errorf("chat session manager is not initialized")
		}
		prefix := sanitizeSessionNamePart(chatName) + "-cont"
		if autoChat {
			prefix = "auto-" + sanitizeSessionNamePart(gf.agent)
		}
		newName, err := nextUniqueSessionName(sessionMgr, prefix)
		if err != nil {
			return err
		}
		chatName = newName
		messages = append([]provider.Message(nil), baseMessages...)
		if autoChat {
			if err := sessionMgr.SetLastAutoSession(gf.agent, chatName); err != nil {
				return err
			}
		}
		fmt.Fprintf(errOut, "[zop] context window reached; continuing in new session %q\n", chatName)
		return nil
	}

	sendPrompt := func(prompt string) error {
		if prompt == "" {
			return nil
		}
		userMessage := provider.Message{Role: "user", Content: prompt}
		for attempt := 0; attempt < 2; attempt++ {
			if gf.verbose {
				fmt.Fprintf(errOut, "[zop] sending text to AI (%d chars)\n", len(prompt))
			}
			currentMessages := append(append([]provider.Message(nil), messages...), userMessage)
			for {
				var tools []provider.Tool
				if useTools {
					tools = registry.List()
				}
				req := provider.CompletionRequest{
					Messages:   currentMessages,
					Model:      modelCfg,
					Stream:     streamFlag,
					StreamFunc: streamFn,
					Tools:      tools,
				}
				resp, rerr := prov.Complete(context.Background(), req)
				if rerr != nil {
					if attempt == 0 && interactive && chatName != "" && sessionMgr != nil && isContextOverflowError(rerr) {
						if err := rolloverSession(); err != nil {
							return fmt.Errorf("rolling over context-limited session: %w", err)
						}
						continue
					}
					return rerr
				}

				if !streamFlag {
					fmt.Fprintln(out, resp.Content)
				} else {
					fmt.Fprintln(out)
				}

				currentMessages = append(currentMessages, provider.Message{
					Role:      "assistant",
					Content:   resp.Content,
					ToolCalls: resp.ToolCalls,
				})

				if len(resp.ToolCalls) == 0 {
					break
				}

				// Execute tool calls
				for _, tc := range resp.ToolCalls {
					if gf.verbose {
						fmt.Fprintf(errOut, "[zop] tool call: %s(%s)\n", tc.Name, tc.Arguments)
					}
					t, ok := registry.Get(tc.Name)
					var toolResult string
					if !ok {
						toolResult = fmt.Sprintf("Error: tool %q not found", tc.Name)
					} else {
						// For run_command, we might want to ask confirmation if not in a special mode,
						// but the user asked for CLI tool calling support.
						res, err := t.Execute(context.Background(), tc.Arguments)
						if err != nil {
							toolResult = fmt.Sprintf("Error: %v", err)
						} else {
							toolResult = res
						}
					}
					currentMessages = append(currentMessages, provider.Message{
						Role:    "tool",
						ToolID:  tc.ID,
						Content: toolResult,
					})
					if gf.verbose {
						fmt.Fprintf(errOut, "[zop] tool result: %d chars\n", len(toolResult))
					}
				}
				if streamFlag {
					fmt.Fprintln(out, "[tool calling...]")
				}
			}

			messages = currentMessages
			if chatName != "" && sessionMgr != nil {
				if err := sessionMgr.Save(chatName, messages); err != nil {
					fmt.Fprintf(errOut, "[zop] warning: could not save session: %v\n", err)
				}
			}
			return nil
		}
		return fmt.Errorf("context rollover retry failed")
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

func sanitizeSessionNamePart(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "session"
	}
	s = sessionPartSanitizer.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_")
	if s == "" {
		return "session"
	}
	return s
}

func nextUniqueSessionName(mgr *chat.Manager, prefix string) (string, error) {
	prefix = sanitizeSessionNamePart(prefix)
	const (
		timePartLen = len("20060102-150405")
		randPartLen = 4
		sepLen      = 2 // prefix-time-rand
		maxNameLen  = 65
	)
	maxPrefix := maxNameLen - timePartLen - randPartLen - sepLen
	if maxPrefix < 1 {
		maxPrefix = 1
	}
	if len(prefix) > maxPrefix {
		prefix = prefix[:maxPrefix]
	}

	for i := 0; i < 16; i++ {
		now := time.Now().UTC()
		candidate := fmt.Sprintf("%s-%s-%04x",
			prefix,
			now.Format("20060102-150405"),
			now.UnixNano()&0xffff,
		)
		exists, err := mgr.Exists(candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		time.Sleep(time.Millisecond)
	}
	return "", fmt.Errorf("could not allocate unique session name for prefix %q", prefix)
}

func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	needles := []string{
		"context length",
		"context window",
		"maximum context",
		"prompt is too long",
		"input is too long",
		"too many tokens",
		"token limit",
		"tokens exceed",
		"request too large",
		"max input tokens",
	}
	for _, needle := range needles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
