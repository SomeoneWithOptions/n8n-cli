// Package confirm guards actions that cannot be undone.
//
// The rule is the same everywhere: an interactive run asks, a non-interactive
// run refuses unless --yes was passed. A missing terminal is never treated as
// consent.
package confirm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrDeclined is returned when the user answers no.
var ErrDeclined = errors.New("aborted")

// ErrNotInteractive is returned when there is no terminal to ask on.
var ErrNotInteractive = errors.New("refusing to continue without confirmation: pass --yes")

// Confirmer asks a yes/no question on a terminal.
//
// In is the input stream, Out the diagnostic stream: questions and answers are
// never part of a command's machine-readable output.
type Confirmer struct {
	In          io.Reader
	Out         io.Writer
	AssumeYes   bool
	Interactive bool
}

// Confirm returns nil when the action may proceed, [ErrDeclined] when the user
// says no, and [ErrNotInteractive] when nobody can be asked.
func (c Confirmer) Confirm(question string) error {
	if c.AssumeYes {
		return nil
	}
	if !c.Interactive || c.In == nil || c.Out == nil {
		return ErrNotInteractive
	}

	fmt.Fprintf(c.Out, "%s [y/N]: ", question)
	line, err := bufio.NewReader(c.In).ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		if err == io.EOF {
			return ErrDeclined
		}
		return fmt.Errorf("read confirmation: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return ErrDeclined
	}
}
