package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleYAML = `openapi: 3.1.0
info:
  title: CLI Test API
  version: 1.0.0
paths:
  /ping:
    get:
      summary: Ping
      responses:
        200:
          description: Pong
          content:
            application/json:
              schema:
                oneOf:
                  - type: string
                  - type: integer
`

func TestCLIFromFileToStdout(t *testing.T) {
	tmp := t.TempDir()
	inPath := filepath.Join(tmp, "spec.yaml")
	if err := os.WriteFile(inPath, []byte(sampleYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetArgs([]string{inPath, "--toc"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"# CLI Test API",
		"## Table of Contents",
		"### GET /ping",
		"one of: string \\| integer (`application/json`)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("CLI output missing %q:\n%s", want, got)
		}
	}
}

func TestCLIFromStdinToOutputFile(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "docs", "api.md")

	root := NewRootCmd()
	root.SetIn(strings.NewReader(sampleYAML))
	root.SetArgs([]string{"-", "-o", outPath, "--no-header", "--title", "Overridden API"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	got := string(b)
	if !strings.HasPrefix(got, "# Overridden API\n") {
		t.Fatalf("unexpected output start: %q", got[:min(50, len(got))])
	}
}

func TestCLIValidateAndVersion(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetIn(strings.NewReader(sampleYAML))
	root.SetOut(&out)
	root.SetArgs([]string{"validate"})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if !strings.Contains(out.String(), "Valid OpenAPI 3.1.0 document: CLI Test API") {
		t.Errorf("unexpected validate output: %q", out.String())
	}

	out.Reset()
	root = NewRootCmd()
	root.SetOut(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}
	if !strings.Contains(out.String(), "openapi2md ") {
		t.Errorf("unexpected version output: %q", out.String())
	}
}

func TestCLIErrorsOnInvalidSpec(t *testing.T) {
	root := NewRootCmd()
	root.SetIn(strings.NewReader(`{"openapi": "3.1.0"}`))
	root.SetArgs([]string{"-"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "info.title") {
		t.Fatalf("expected info.title error, got %v", err)
	}
}

func TestCLIHelpIncludesExamples(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("--help failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Examples:",
		"openapi2md openapi.yaml",
		"openapi2md openapi.json --toc -o docs/API.md",
		"openapi2md - < openapi.json",
		"openapi2md validate openapi.yaml",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("root help output missing %q:\n%s", want, got)
		}
	}

	out.Reset()
	root = NewRootCmd()
	root.SetOut(&out)
	root.SetArgs([]string{"validate", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate --help failed: %v", err)
	}

	got = out.String()
	for _, want := range []string{
		"Examples:",
		"openapi2md validate openapi.yaml",
		"cat openapi.json | openapi2md validate",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("validate help output missing %q:\n%s", want, got)
		}
	}
}
