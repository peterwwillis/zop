package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/peterwwillis/pgpt/internal/chat"
)

func newChatCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Manage chat sessions",
	}
	cmd.AddCommand(newChatListCmd(gf))
	cmd.AddCommand(newChatShowCmd(gf))
	cmd.AddCommand(newChatDeleteCmd(gf))
	return cmd
}

func newChatListCmd(_ *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all chat sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr, err := chat.NewManager("")
			if err != nil {
				return err
			}
			names, err := mgr.List()
			if err != nil {
				return err
			}
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No chat sessions found.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SESSION")
			for _, n := range names {
				fmt.Fprintln(w, n)
			}
			return w.Flush()
		},
	}
}

func newChatShowCmd(_ *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "show <session>",
		Short: "Show the messages in a chat session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := chat.NewManager("")
			if err != nil {
				return err
			}
			msgs, err := mgr.Get(args[0])
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Session is empty or does not exist.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ROLE\tCONTENT")
			for _, m := range msgs {
				content := m.Content
				if len(content) > 80 {
					content = content[:77] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\n", m.Role, content)
			}
			return w.Flush()
		},
	}
}

func newChatDeleteCmd(_ *globalFlags) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <session>",
		Short: "Delete a chat session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := chat.NewManager("")
			if err != nil {
				return err
			}
			if !force {
				fmt.Fprintf(os.Stderr, "Delete session %q? [y/N] ", args[0])
				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					fmt.Fprintln(os.Stderr, "Could not read input; aborting.")
					return nil
				}
				if answer != "y" && answer != "Y" {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			if err := mgr.Delete(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Session %q deleted.\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation")
	return cmd
}
