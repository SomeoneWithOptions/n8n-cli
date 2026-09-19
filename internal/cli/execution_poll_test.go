package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// autoTicks replaces the poll ticker with one that fires as soon as the loop
// is ready for it, so polling tests run without real time passing. The real
// ticker is restored when the test ends.
func autoTicks(t *testing.T) {
	t.Helper()
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case ticks <- time.Time{}:
			case <-done:
				return
			}
		}
	}()
	orig := tick
	tick = func(time.Duration) (<-chan time.Time, func()) { return ticks, func() {} }
	t.Cleanup(func() {
		tick = orig
		close(done)
	})
}

func TestValidatePollInterval(t *testing.T) {
	for _, d := range []time.Duration{time.Second, 2 * time.Second, time.Hour} {
		if err := validatePollInterval(d); err != nil {
			t.Errorf("%s: unexpected error %v", d, err)
		}
	}
	if err := validatePollInterval(500 * time.Millisecond); err == nil || !strings.Contains(err.Error(), "at least 1s") {
		t.Errorf("500ms error = %v, want the lower bound named", err)
	}
	if err := validatePollInterval(2 * time.Hour); err == nil || !strings.Contains(err.Error(), "at most 1h") {
		t.Errorf("2h error = %v, want the upper bound named", err)
	}
}

func TestPollRetriesTransientAndStopsOnFatal(t *testing.T) {
	autoTicks(t)
	var errOut strings.Builder
	calls := 0
	err := poll(context.Background(), time.Second, &errOut, func(context.Context) (bool, error) {
		calls++
		switch calls {
		case 1:
			return false, nil
		case 2:
			return false, errors.New("connection reset")
		case 3:
			return false, &n8n.APIError{StatusCode: 401}
		}
		t.Fatal("poll continued after a fatal error")
		return true, nil
	})
	if !n8n.IsUnauthorized(err) {
		t.Errorf("err = %v, want the 401 surfaced", err)
	}
	if !strings.Contains(errOut.String(), "Warning: poll failed, retrying in 1s: connection reset") {
		t.Errorf("stderr = %q, want the transient failure reported", errOut.String())
	}
}

func TestPollFirstFailureIsFatalAndCancelIsSuccess(t *testing.T) {
	autoTicks(t)
	var errOut strings.Builder
	boom := errors.New("boom")
	err := poll(context.Background(), time.Second, &errOut, func(context.Context) (bool, error) { return false, boom })
	if !errors.Is(err, boom) {
		t.Errorf("first failure err = %v, want %v", err, boom)
	}

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err = poll(ctx, time.Second, &errOut, func(context.Context) (bool, error) {
		calls++
		if calls == 2 {
			cancel()
			return false, context.Canceled
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("canceled poll err = %v, want nil", err)
	}
}

func TestExecutionIDLess(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{{"9", "10", true}, {"10", "9", false}, {"10", "10", false}, {"abc", "abd", true}, {"9", "x", true}} {
		if got := executionIDLess(tc.a, tc.b); got != tc.want {
			t.Errorf("executionIDLess(%q, %q) = %t, want %t", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestExecutionDurationMs(t *testing.T) {
	start, stop := "2026-09-15T22:27:14.371Z", "2026-09-15T22:27:19.511Z"
	if got := executionDurationMs(&n8n.Execution{Status: "success", StartedAt: &start, StoppedAt: &stop}); got == nil || *got != 5140 {
		t.Errorf("duration = %v, want 5140", got)
	}
	if got := executionDurationMs(&n8n.Execution{StartedAt: &start}); got != nil {
		t.Errorf("running duration = %v, want nil", got)
	}
	waiting := &n8n.Execution{Status: "waiting", StartedAt: &start, StoppedAt: &stop}
	if executionDurationMs(waiting) != nil || executionStoppedAt(waiting) != nil {
		t.Error("a waiting run's stoppedAt must not count as finished")
	}
	bad := "yesterday"
	if got := executionDurationMs(&n8n.Execution{Status: "success", StartedAt: &bad, StoppedAt: &stop}); got != nil {
		t.Errorf("unparseable duration = %v, want nil", got)
	}
	if formatDurationMs(nil) != "-" {
		t.Error("nil duration must render as a dash")
	}
}
