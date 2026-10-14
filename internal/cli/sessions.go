package cli

import (
	"fmt"
	"io"

	"github.com/Godric-W/Equus/internal/threadstore"
	"github.com/spf13/cobra"
)

func newSessionsCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "sessions",
		Short: "List saved chat sessions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			store := threadstore.New(sessionsDir())
			metas, err := store.List()
			if err != nil {
				return err
			}
			if len(metas) == 0 {
				fmt.Fprintln(stdout, "no saved sessions")
				return nil
			}
			for _, meta := range metas {
				fmt.Fprintf(stdout, "%s  %s  %s\n",
					meta.ID, meta.CreatedAt.Format("2006-01-02 15:04"), meta.CWD)
			}
			return nil
		},
	}
}
