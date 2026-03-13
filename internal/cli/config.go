package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/peterwwillis/pgpt/internal/config"
)

func newConfigCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or validate the current configuration",
	}
	cmd.AddCommand(newConfigShowCmd(gf))
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
			for name, a := range cfg.Agents {
				fmt.Fprintf(w, "  %-20s provider=%-12s model=%s\n", name, a.Provider, a.Model)
			}
			fmt.Fprintln(w, "=== Providers ===")
			for name, p := range cfg.Providers {
				fmt.Fprintf(w, "  %-20s api_key_env=%-25s base_url=%s\n", name, p.APIKeyEnv, p.BaseURL)
			}
			fmt.Fprintln(w, "=== Models ===")
			for name, m := range cfg.Models {
				fmt.Fprintf(w, "  %-20s id=%-30s max_tokens=%d temp=%.2f top_p=%.2f\n",
					name, m.ModelID, m.MaxTokens, m.Temperature, m.TopP)
			}
			return nil
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
