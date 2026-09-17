// Package docs renders the command reference under docs/.
//
// Cobra help is the source of truth: everything here is generated from Short,
// Long, Example and flag usage, so documentation cannot drift from the binary.
// Fix a wrong page by fixing the command's help text.
package docs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// Dir is the command reference directory, relative to the repository root.
const Dir = "docs"

// indexName is the entry point page of the generated reference.
const indexName = "README.md"

// Render walks the command tree and returns the reference as file name to
// content, without touching the filesystem, so a test can compare it with what
// is committed.
func Render(root *cobra.Command) (map[string][]byte, error) {
	prepare(root)

	files := map[string][]byte{}
	var walk func(cmd *cobra.Command) error
	walk = func(cmd *cobra.Command) error {
		if !cmd.IsAvailableCommand() && cmd != root {
			return nil
		}
		var buf bytes.Buffer
		if err := doc.GenMarkdownCustom(cmd, &buf, link); err != nil {
			return fmt.Errorf("render %q: %w", cmd.CommandPath(), err)
		}
		files[fileName(cmd)] = buf.Bytes()
		for _, sub := range cmd.Commands() {
			if err := walk(sub); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	files[indexName] = index(root)
	return files, nil
}

// Write generates the reference into dir, replacing the pages that changed and
// deleting pages for commands that no longer exist.
func Write(root *cobra.Command, dir string) error {
	files, err := Render(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	existing, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return err
	}
	for _, path := range existing {
		if _, keep := files[filepath.Base(path)]; !keep {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// prepare makes generation deterministic: no timestamp footer, and no help
// command page, because the reference is the help.
func prepare(root *cobra.Command) {
	root.DisableAutoGenTag = true
	root.InitDefaultHelpCmd()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "help" {
			cmd.Hidden = true
		}
	}
}

// fileName is the page of one command: "n8n discover" becomes n8n_discover.md,
// which is the layout cobra's own link handler expects.
func fileName(cmd *cobra.Command) string {
	return strings.ReplaceAll(cmd.CommandPath(), " ", "_") + ".md"
}

func link(name string) string { return name }

// index lists every command in one page, so a reader (or an agent) can find a
// command without opening each file.
func index(root *cobra.Command) []byte {
	var rows []string
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if !cmd.IsAvailableCommand() && cmd != root {
			return
		}
		rows = append(rows, fmt.Sprintf("| [`%s`](%s) | %s |", cmd.CommandPath(), fileName(cmd), cmd.Short))
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(root)
	sort.Strings(rows)

	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s command reference\n\n", root.Name())
	fmt.Fprintf(&b, "Generated from the command help. Do not edit these files by hand:\n")
	fmt.Fprintf(&b, "edit the command's Short, Long, Example or flag usage and run `make docs`.\n\n")
	fmt.Fprintf(&b, "| Command | Description |\n|---|---|\n")
	for _, row := range rows {
		fmt.Fprintln(&b, row)
	}
	return b.Bytes()
}
