package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// errNoTerminal is returned when a secret is needed and there is nothing to
// prompt on. A credential is never accepted as a flag or an argument, so the
// caller is pointed at the stdin path instead.
var errNoTerminal = errors.New("no terminal available to read a credential: pipe it with --stdin")

// SecretPrompter reads credential material without echoing it.
//
// It is an interface so authentication tests run without a TTY; the terminal
// implementation is the only one that touches the real console.
type SecretPrompter interface {
	PromptSecret(prompt string) (n8n.Secret, error)
}

// terminalPrompter reads from the controlling terminal with echo disabled.
type terminalPrompter struct {
	in  io.Reader
	out io.Writer
}

func (p terminalPrompter) PromptSecret(prompt string) (n8n.Secret, error) {
	f, ok := p.in.(*os.File)
	if !ok {
		return "", errNoTerminal
	}
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		return "", errNoTerminal
	}

	fmt.Fprint(p.out, prompt)
	data, err := term.ReadPassword(fd)
	// The newline the user typed was swallowed with the echo.
	fmt.Fprintln(p.out)
	if err != nil {
		return "", fmt.Errorf("read credential: %w", err)
	}
	defer clear(data)

	secret := n8n.Secret(strings.TrimRight(string(data), "\r\n"))
	if secret.Empty() {
		return "", errors.New("credential is empty")
	}
	return secret, nil
}

// maxSecretStdin caps a credential piped on stdin. One byte more is read so
// an overlong input fails instead of silently authenticating with a prefix.
const maxSecretStdin = 1 << 20

// readSecretFrom consumes a credential piped on stdin. Only a trailing newline
// is stripped: everything else is credential material.
func readSecretFrom(r io.Reader) (n8n.Secret, error) {
	if r == nil {
		return "", errors.New("no stdin to read a credential from")
	}
	data, err := io.ReadAll(io.LimitReader(r, maxSecretStdin+1))
	if err != nil {
		return "", fmt.Errorf("read credential from stdin: %w", err)
	}
	defer clear(data)
	if len(data) > maxSecretStdin {
		return "", fmt.Errorf("credential on stdin exceeds %d bytes", maxSecretStdin)
	}
	secret := n8n.Secret(strings.TrimRight(string(data), "\r\n"))
	if secret.Empty() {
		return "", errors.New("credential on stdin is empty")
	}
	return secret, nil
}

// promptLine reads one echoed, non-secret line, such as an instance URL.
func promptLine(in io.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// isTerminal reports whether a stream is a terminal. It uses the same test as
// the credential prompt, so interactivity and prompting agree: /dev/null is a
// character device but not a terminal, and must fail closed in both.
func isTerminal(stream any) bool {
	f, ok := stream.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
