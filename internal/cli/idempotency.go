package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// idempotencyAuto asks for a generated key instead of a literal one.
const idempotencyAuto = "auto"

// idempotencyHelp is the sentence a command's Long adds for the flag.
const idempotencyHelp = "--idempotency-key KEY makes a retry after a timeout or lost response safe:\n" +
	"the same request with the same key returns the first result instead of\n" +
	"running again (auto generates a key and prints it on stderr)."

// idempotencyFlag is the --idempotency-key flag of a POST command whose repeat
// would start work or write content twice. The CLI never retries on its own:
// the key only makes the caller's own retry safe.
type idempotencyFlag struct {
	key string
}

func (f *idempotencyFlag) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.key, "idempotency-key", "", "retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)")
}

// validate checks a literal key before any prompt or request.
func (f idempotencyFlag) validate() error {
	if f.key == "" || f.key == idempotencyAuto {
		return nil
	}
	if err := n8n.ValidateIdempotencyKey(f.key); err != nil {
		return fmt.Errorf("--idempotency-key: %w", err)
	}
	return nil
}

// resolve turns the flag into call options right before the request. auto
// becomes a fresh key, announced on stderr first so an interrupted or failed
// run can be retried with it.
func (f *idempotencyFlag) resolve(stderr io.Writer) []n8n.CallOption {
	switch f.key {
	case "":
		return nil
	case idempotencyAuto:
		f.key = n8n.NewIdempotencyKey()
		fmt.Fprintf(stderr, "Idempotency key: %s (to retry this exact request, rerun with --idempotency-key %s)\n", f.key, f.key)
	}
	return []n8n.CallOption{n8n.IdempotencyKey(f.key)}
}

// refusal reports the two failures the key itself causes, or nil when err is
// something else and the command's own error handling applies. It runs before
// that handling because the same status codes also mean resource conflicts.
func (f idempotencyFlag) refusal(err error, resolution config.Resolution) error {
	if f.key == "" {
		return nil
	}
	switch {
	case n8n.IsIdempotencyMismatch(err):
		return fmt.Errorf("%s refused idempotency key %q (422): the key was already used with a different request; rerun with a new key, e.g. --idempotency-key auto\n  server said: %w", resolution.URL, f.key, err)
	case n8n.IsIdempotencyInProgress(err):
		return fmt.Errorf("%s is still running the original request for idempotency key %q (409): retry later with the same key to get its result without running it twice\n  server said: %w", resolution.URL, f.key, err)
	}
	return nil
}
