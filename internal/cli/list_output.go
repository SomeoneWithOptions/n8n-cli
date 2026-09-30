package cli

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/spf13/cobra"
)

// scriptViewHelp is the shared Long paragraph for list commands that offer
// --quiet and --brief; columns names the --brief fields in order.
func scriptViewHelp(noun, columns string) string {
	return "Script views print rows only, with no header, count or blank lines, so stdout\n" +
		"pipes cleanly; the next cursor and any empty-result hint go to stderr, and an\n" +
		"empty result prints nothing:\n" +
		"  --quiet  one " + noun + " ID per line, printed exactly\n" +
		"  --brief  " + columns + ", separated by a literal tab\n" +
		"--brief escapes backslash, tab, newline, carriage return and other control\n" +
		"characters in fields as \\\\, \\t, \\n, \\r and \\uXXXX; use --output json for exact\n" +
		"values. The views combine neither with each other nor with --output json."
}

// listViewFlags are the header-free script views a core list command offers
// next to its default table.
type listViewFlags struct {
	quiet bool
	brief bool
}

// register adds --quiet and --brief to one list command; columns names the
// --brief fields in order.
func (f *listViewFlags) register(cmd *cobra.Command, noun, columns string) {
	cmd.Flags().BoolVar(&f.quiet, "quiet", false, "print only "+noun+" IDs, one per line, with no header (default: full table)")
	cmd.Flags().BoolVar(&f.brief, "brief", false, "print only "+columns+", tab-separated and escaped, with no header (default: full table)")
}

func (f listViewFlags) enabled() bool { return f.quiet || f.brief }

// validate checks --output and the view flags together, before any request.
func (f listViewFlags) validate(output string) error {
	if err := validateOutput(output); err != nil {
		return err
	}
	if f.quiet && f.brief {
		return fmt.Errorf("--quiet cannot be combined with --brief: --quiet prints IDs only, --brief prints IDs and a few fields")
	}
	if f.enabled() && output == outputJSON {
		return fmt.Errorf("--quiet and --brief cannot be combined with --output json: they are text views; extract fields from the JSON with jq instead")
	}
	return nil
}

// scriptListRow is one script-view row: an exact ID, then the --brief fields.
type scriptListRow struct {
	ID     string
	Fields []string
}

// writeScriptPage renders one fetched page as a script view: rows on stdout,
// then the next-cursor hint and the empty-result hint on stderr.
func writeScriptPage[T any](opts Options, view listViewFlags, noun string, page n8n.Page[T], emptyHint string, row func(T) scriptListRow) error {
	if err := writeScriptList(opts.Streams.Out, noun, page.Data, view.quiet, row); err != nil {
		return err
	}
	if page.HasMore() {
		fmt.Fprintf(opts.Streams.Err, "More %ss: pass --cursor %s for the next page, or use --all.\n", noun, escapeScriptField(page.NextCursor))
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, emptyHint)
	}
	return nil
}

// writeScriptList writes one ID per line (idsOnly) or tab-separated escaped
// rows. Every ID is checked before the first byte is written, so a malformed
// row never leaves a partial inventory on stdout.
func writeScriptList[T any](out io.Writer, noun string, items []T, idsOnly bool, row func(T) scriptListRow) error {
	var b strings.Builder
	for _, item := range items {
		r := row(item)
		if err := validateScriptID(noun, r.ID); err != nil {
			return err
		}
		b.WriteString(r.ID)
		if !idsOnly {
			for _, field := range r.Fields {
				b.WriteByte('\t')
				b.WriteString(escapeScriptField(field))
			}
		}
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return nil
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// validateScriptID refuses an ID that would print as a different ID or as an
// extra row: an ID is printed raw so other commands accept it verbatim.
func validateScriptID(noun, id string) error {
	if id == "" {
		return fmt.Errorf("the instance returned a %s with an empty ID, which cannot be printed as a script row; use --output json instead", noun)
	}
	if strings.IndexFunc(id, isScriptControl) >= 0 {
		return fmt.Errorf("the instance returned %s ID %q containing a control character, which cannot be printed as a script row; use --output json instead", noun, id)
	}
	return nil
}

// escapeScriptField keeps a --brief field on one line and inside one column.
// Backslash is escaped too, so a literal "\n" stays distinct from a newline.
func escapeScriptField(value string) string {
	if strings.IndexFunc(value, func(r rune) bool { return r == '\\' || isScriptControl(r) }) < 0 {
		return value
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case isScriptControl(r):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isScriptControl reports C0, DEL and C1 controls plus the Unicode line and
// paragraph separators, all of which can break rows or drive a terminal.
func isScriptControl(r rune) bool {
	return unicode.IsControl(r) || r == '\u2028' || r == '\u2029'
}
