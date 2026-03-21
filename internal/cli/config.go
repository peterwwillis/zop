package cli

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/peterwwillis/zop/internal/config"
)

func newConfigCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the zop configuration file",
	}
	cmd.AddCommand(newConfigShowCmd(gf))
	cmd.AddCommand(newConfigListCmd(gf))
	cmd.AddCommand(newConfigGetCmd(gf))
	cmd.AddCommand(newConfigSetCmd(gf))
	cmd.AddCommand(newConfigUnsetCmd(gf))
	cmd.AddCommand(newConfigRemoveCmd(gf))
	cmd.AddCommand(newConfigEditCmd(gf))
	cmd.AddCommand(newConfigPathCmd())
	return cmd
}

func newConfigShowCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the resolved configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(gf.configFile)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "=== Agents ===")
			for _, name := range sortedKeys(cfg.Agents) {
				a := cfg.Agents[name]
				fmt.Fprintf(w, "  %-20s provider=%-12s model=%s", name, a.Provider, a.Model)
				if a.SystemPrompt != "" {
					fmt.Fprintf(w, " system_prompt=%q", a.SystemPrompt)
				}
				if a.ToolPolicy != nil {
					fmt.Fprintf(w, " has_tool_policy=true")
				}
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w, "=== Providers ===")
			for _, name := range sortedKeys(cfg.Providers) {
				p := cfg.Providers[name]
				fmt.Fprintf(w, "  %-20s api_key_env=%-25s base_url=%s\n", name, p.APIKeyEnv, p.BaseURL)
			}
			fmt.Fprintln(w, "=== Models ===")
			for _, name := range sortedKeys(cfg.Models) {
				m := cfg.Models[name]
				fmt.Fprintf(w, "  %-20s id=%-30s max_tokens=%d temp=%.2f top_p=%.2f",
					name, m.ModelID, m.MaxTokens, m.Temperature, m.TopP)
				if m.TopK != 0 {
					fmt.Fprintf(w, " top_k=%d", m.TopK)
				}
				if m.RepeatPenalty != 0 {
					fmt.Fprintf(w, " repeat_penalty=%.2f", m.RepeatPenalty)
				}
				if m.SystemPrompt != "" {
					fmt.Fprintf(w, " system_prompt=%q", m.SystemPrompt)
				}
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w, "=== MCP Servers ===")
			for _, name := range sortedKeys(cfg.MCPServers) {
				s := cfg.MCPServers[name]
				if s.URL != "" {
					fmt.Fprintf(w, "  %-20s url=%s\n", name, s.URL)
				} else {
					fmt.Fprintf(w, "  %-20s command=%s args=%v\n", name, s.Command, s.Args)
				}
			}
			fmt.Fprintln(w, "=== Global Tool Policy ===")
			fmt.Fprintf(w, "  allow_list: %d entries\n", len(cfg.ToolPolicy.AllowList))
			fmt.Fprintf(w, "  deny_list:  %d entries\n", len(cfg.ToolPolicy.DenyList))
			fmt.Fprintf(w, "  allow_tags: %v\n", cfg.ToolPolicy.AllowTags)
			fmt.Fprintf(w, "  deny_tags:  %v\n", cfg.ToolPolicy.DenyTags)

			return nil
		},
	}
}

func newConfigListCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list [section]",
		Short: "List configuration entries",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := config.LoadRaw(gf.configFile)
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			if len(args) == 1 {
				_, entries, err := sectionEntries(raw, args[0])
				if err != nil {
					return err
				}
				if len(entries) == 0 {
					return nil
				}
				for _, name := range entries {
					fmt.Fprintln(w, name)
				}
				return nil
			}

			for _, section := range configSections() {
				_, entries, err := sectionEntries(raw, section)
				if err != nil {
					return err
				}
				for _, name := range entries {
					fmt.Fprintf(w, "%s.%s\n", section, name)
				}
			}
			return nil
		},
	}
}

func newConfigGetCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "get <section.name[.field]>",
		Short: "Get a configuration value or section",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			section, name, field, err := parseConfigPath(args[0])
			if err != nil {
				return err
			}
			raw, err := config.LoadRaw(gf.configFile)
			if err != nil {
				return err
			}
			sectionMap, err := configSection(raw, section)
			if err != nil {
				return err
			}
			entry, ok := sectionMap[name]
			if !ok {
				return fmt.Errorf("%s.%s not found", section, name)
			}

			w := cmd.OutOrStdout()
			if field == "" {
				keys := sortedFieldKeys(entry)
				for _, key := range keys {
					fmt.Fprintf(w, "%s=%v\n", key, entry[key])
				}
				return nil
			}
			value, ok := entry[field]
			if !ok {
				return fmt.Errorf("%s.%s.%s not found", section, name, field)
			}
			fmt.Fprintln(w, value)
			return nil
		},
	}
}

func newConfigSetCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "set <section.name.field> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			section, name, field, err := parseConfigPath(args[0])
			if err != nil {
				return err
			}
			if field == "" {
				return fmt.Errorf("config set requires a field path")
			}
			raw, err := config.LoadRaw(gf.configFile)
			if err != nil {
				return err
			}
			sectionMap, err := configSection(raw, section)
			if err != nil {
				return err
			}
			entry, ok := sectionMap[name]
			if !ok {
				entry = map[string]interface{}{}
				sectionMap[name] = entry
			}
			entry[field] = parseConfigValue(args[1])
			return config.WriteRaw(gf.configFile, raw)
		},
	}
}

func newConfigUnsetCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <section.name.field>",
		Short: "Unset a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			section, name, field, err := parseConfigPath(args[0])
			if err != nil {
				return err
			}
			if field == "" {
				return fmt.Errorf("config unset requires a field path")
			}
			raw, err := config.LoadRaw(gf.configFile)
			if err != nil {
				return err
			}
			sectionMap, err := configSection(raw, section)
			if err != nil {
				return err
			}
			entry, ok := sectionMap[name]
			if !ok {
				return fmt.Errorf("%s.%s not found", section, name)
			}
			if _, ok := entry[field]; !ok {
				return fmt.Errorf("%s.%s.%s not found", section, name, field)
			}
			delete(entry, field)
			return config.WriteRaw(gf.configFile, raw)
		},
	}
}

func newConfigRemoveCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <section.name>",
		Short: "Remove a configuration entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			section, name, field, err := parseConfigPath(args[0])
			if err != nil {
				return err
			}
			if field != "" {
				return fmt.Errorf("config remove only accepts section.name")
			}
			raw, err := config.LoadRaw(gf.configFile)
			if err != nil {
				return err
			}
			sectionMap, err := configSection(raw, section)
			if err != nil {
				return err
			}
			if _, ok := sectionMap[name]; !ok {
				return fmt.Errorf("%s.%s not found", section, name)
			}
			delete(sectionMap, name)
			return config.WriteRaw(gf.configFile, raw)
		},
	}
}

func newConfigEditCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Edit the configuration file in $EDITOR",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.EnsureConfigFile(gf.configFile)
			if err != nil {
				return err
			}
			editor := os.Getenv("VISUAL")
			if editor == "" {
				editor = os.Getenv("EDITOR")
			}
			parts := strings.Fields(editor)
			if len(parts) == 0 {
				return fmt.Errorf("EDITOR is not set")
			}
			editorCmd := exec.Command(parts[0], append(parts[1:], path)...)
			editorCmd.Stdin = cmd.InOrStdin()
			editorCmd.Stdout = cmd.OutOrStdout()
			editorCmd.Stderr = cmd.ErrOrStderr()
			return editorCmd.Run()
		},
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the default config file path",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), config.DefaultConfigPath())
			return nil
		},
	}
}

func configSections() []string {
	return []string{"agents", "providers", "models", "mcp_servers", "tool_policy"}
}

func configSection(raw config.RawConfig, section string) (map[string]map[string]interface{}, error) {
	section = strings.ToLower(section)
	sectionMap, ok := raw[section]
	if !ok {
		return nil, fmt.Errorf("unknown config section %q", section)
	}
	return sectionMap, nil
}

func sectionEntries(raw config.RawConfig, section string) (string, []string, error) {
	sectionMap, err := configSection(raw, section)
	if err != nil {
		return "", nil, err
	}
	keys := sortedKeys(sectionMap)
	return section, keys, nil
}

func parseConfigPath(path string) (string, string, string, error) {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid config path %q (expected section.name[.field])", path)
	}
	section := strings.ToLower(parts[0])
	name := parts[1]
	field := ""
	if len(parts) > 2 {
		field = strings.Join(parts[2:], ".")
	}
	return section, name, field, nil
}

func parseConfigValue(value string) interface{} {
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal
	}
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}
	return value
}

func sortedFieldKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
