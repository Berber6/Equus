package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Version is the Equus build version, overridable via -ldflags.
var Version = "0.1.0"

func newVersionCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Equus version",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			fmt.Fprintln(stdout, "equus "+Version)
			return nil
		},
	}
}
