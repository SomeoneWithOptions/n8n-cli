package confirm

import (
	"errors"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		assumeYes   bool
		interactive bool
		wantErr     error
	}{
		{name: "yes", input: "y\n", interactive: true},
		{name: "yes spelled out", input: "YES\n", interactive: true},
		{name: "padded", input: "  y  \n", interactive: true},
		{name: "no", input: "n\n", interactive: true, wantErr: ErrDeclined},
		{name: "empty line defaults to no", input: "\n", interactive: true, wantErr: ErrDeclined},
		{name: "anything else is no", input: "sure\n", interactive: true, wantErr: ErrDeclined},
		{name: "closed stdin is no", input: "", interactive: true, wantErr: ErrDeclined},
		{name: "answer without newline", input: "y", interactive: true},
		{name: "not interactive", input: "y\n", wantErr: ErrNotInteractive},
		{name: "assume yes skips the question", assumeYes: true},
		{name: "assume yes without a terminal", assumeYes: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			c := Confirmer{
				In:          strings.NewReader(tt.input),
				Out:         &out,
				AssumeYes:   tt.assumeYes,
				Interactive: tt.interactive,
			}

			err := c.Confirm("Delete everything?")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Confirm() = %v, want %v", err, tt.wantErr)
			}
			asked := strings.Contains(out.String(), "Delete everything? [y/N]: ")
			if wantAsked := tt.interactive && !tt.assumeYes; asked != wantAsked {
				t.Errorf("question asked = %v, want %v (output %q)", asked, wantAsked, out.String())
			}
		})
	}
}

func TestConfirmWithoutInput(t *testing.T) {
	var out strings.Builder
	err := Confirmer{Out: &out, Interactive: true}.Confirm("Proceed?")
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("Confirm() = %v, want ErrNotInteractive", err)
	}
}
