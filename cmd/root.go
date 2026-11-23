package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/oxio/openapi2md/internal/markdown"
	"github.com/oxio/openapi2md/internal/openapi"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// NewRootCmd constructs the root Cobra command for openapi2md.
func NewRootCmd() *cobra.Command {
	var (
		outputPath string
		opts       markdown.Options
	)

	rootCmd := &cobra.Command{
		Use:   "openapi2md [input]",
		Short: "Convert OpenAPI 3.0/3.1 JSON or YAML specifications to Markdown",
		Long: `openapi2md converts OpenAPI 3.0 and 3.1 specifications (JSON or YAML)
into clean, deterministic GitHub-flavored Markdown documentation.

Pass a file path as [input], or use "-" (or omit the argument when piping)
to read the OpenAPI specification from standard input.`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := readInput(cmd, args)
			if err != nil {
				return err
			}

			doc, err := openapi.Parse(data)
			if err != nil {
				return err
			}

			rendered, err := markdown.Render(doc, opts)
			if err != nil {
				return err
			}

			return writeOutput(cmd, outputPath, rendered)
		},
	}

	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output Markdown file path (defaults to stdout)")
	rootCmd.Flags().StringVar(&opts.Title, "title", "", "Override the document title from info.title")
	rootCmd.Flags().StringVar(&opts.SpecURL, "spec-url", "", "Optional OpenAPI specification URL or path to mention in the header")
	rootCmd.Flags().StringVar(&opts.Header, "header", markdown.DefaultHeader, "Header comment emitted at the top of the Markdown document")
	rootCmd.Flags().BoolVar(&opts.NoHeader, "no-header", false, "Omit the generated-file header comment")
	rootCmd.Flags().BoolVar(&opts.TOC, "toc", false, "Include a Table of Contents")
	rootCmd.Flags().BoolVar(&opts.NoSchemas, "no-schemas", false, "Omit the Schemas section")

	rootCmd.AddCommand(newValidateCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

// Execute runs the root CLI command and exits with a non-zero code on error.
func Execute() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func readInput(cmd *cobra.Command, args []string) ([]byte, error) {
	if len(args) == 0 || args[0] == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("no input provided: pass an OpenAPI file path or pipe specification via stdin")
		}
		return data, nil
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", args[0], err)
	}
	return data, nil
}

func writeOutput(cmd *cobra.Command, outputPath, content string) error {
	if outputPath == "" || outputPath == "-" {
		_, err := io.WriteString(cmd.OutOrStdout(), content)
		return err
	}
	if dir := filepath.Dir(outputPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create output directory %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(outputPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", outputPath, err)
	}
	return nil
}
