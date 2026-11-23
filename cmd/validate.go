package cmd

import (
	"fmt"

	"github.com/oxio/openapi2md/internal/markdown"
	"github.com/oxio/openapi2md/internal/openapi"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [input]",
		Short: "Validate an OpenAPI JSON or YAML specification",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := readInput(cmd, args)
			if err != nil {
				return err
			}
			doc, err := openapi.Parse(data)
			if err != nil {
				return err
			}
			// Dry-run render to ensure all operations decode cleanly.
			if _, err := markdown.Render(doc, markdown.Options{}); err != nil {
				return err
			}
			schemaCount := 0
			if doc.Components != nil {
				schemaCount = len(doc.Components.Schemas)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Valid OpenAPI %s document: %s (%d paths, %d webhooks, %d schemas)\n",
				doc.OpenAPI, doc.Info.Title, len(doc.Paths), len(doc.Webhooks), schemaCount)
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the openapi2md version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "openapi2md %s\n", Version)
		},
	}
}
