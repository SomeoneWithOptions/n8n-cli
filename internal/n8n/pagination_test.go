package n8n

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestListOptionsApply(t *testing.T) {
	tests := []struct {
		name string
		in   url.Values
		opts ListOptions
		want string
	}{
		{"zero options", nil, ListOptions{}, ""},
		{"limit only", nil, ListOptions{Limit: 50}, "limit=50"},
		{"cursor only", nil, ListOptions{Cursor: "abc"}, "cursor=abc"},
		{"both", nil, ListOptions{Limit: 10, Cursor: "abc"}, "cursor=abc&limit=10"},
		{"cursor escaped", nil, ListOptions{Cursor: "a b&c"}, "cursor=a+b%26c"},
		{"merged with existing", url.Values{"active": {"true"}}, ListOptions{Limit: 10}, "active=true&limit=10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.Apply(tt.in).Encode(); got != tt.want {
				t.Errorf("Apply().Encode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListOptionsApplyDoesNotMutateNilInput(t *testing.T) {
	got := ListOptions{Limit: 5}.Apply(nil)
	if got == nil {
		t.Fatal("Apply(nil) = nil, want fresh values")
	}
	if got.Get("limit") != "5" {
		t.Errorf("limit = %q, want 5", got.Get("limit"))
	}
}

func TestListOptionsValidate(t *testing.T) {
	if err := (ListOptions{Limit: 0}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil for the server default", err)
	}
	if err := (ListOptions{Limit: -1}).Validate(); err == nil {
		t.Error("Validate() = nil, want an error for a negative limit")
	}
}

func TestPageHasMore(t *testing.T) {
	if (Page[int]{NextCursor: "next"}).HasMore() != true {
		t.Error("HasMore() = false, want true")
	}
	if (Page[int]{}).HasMore() != false {
		t.Error("HasMore() = true, want false")
	}
}

// pager serves fixed pages and records the options it was called with.
type pager struct {
	pages []Page[string]
	calls []ListOptions
}

func (p *pager) fetch(_ context.Context, opts ListOptions) (Page[string], error) {
	p.calls = append(p.calls, opts)
	index := 0
	if opts.Cursor != "" {
		if _, err := fmt.Sscanf(opts.Cursor, "cursor-%d", &index); err != nil {
			return Page[string]{}, fmt.Errorf("unexpected cursor %q", opts.Cursor)
		}
	}
	return p.pages[index], nil
}

func TestCollectWalksEveryPage(t *testing.T) {
	p := &pager{pages: []Page[string]{
		{Data: []string{"a", "b"}, NextCursor: "cursor-1"},
		{Data: []string{"c", "d"}, NextCursor: "cursor-2"},
		{Data: []string{"e"}},
	}}

	got, err := Collect(context.Background(), p.fetch, ListOptions{Limit: 2}, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if want := "a,b,c,d,e"; strings.Join(got, ",") != want {
		t.Errorf("Collect() = %v, want %s", got, want)
	}
	if len(p.calls) != 3 {
		t.Fatalf("fetched %d pages, want 3", len(p.calls))
	}
	for i, call := range p.calls {
		if call.Limit != 2 {
			t.Errorf("call %d limit = %d, want the limit carried through", i, call.Limit)
		}
	}
	if p.calls[0].Cursor != "" || p.calls[1].Cursor != "cursor-1" || p.calls[2].Cursor != "cursor-2" {
		t.Errorf("cursors = %v, want them chained", p.calls)
	}
}

func TestCollectStopsAtMax(t *testing.T) {
	p := &pager{pages: []Page[string]{
		{Data: []string{"a", "b"}, NextCursor: "cursor-1"},
		{Data: []string{"c", "d"}, NextCursor: "cursor-2"},
		{Data: []string{"e"}},
	}}

	got, err := Collect(context.Background(), p.fetch, ListOptions{}, 3)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if want := "a,b,c"; strings.Join(got, ",") != want {
		t.Errorf("Collect() = %v, want %s", got, want)
	}
	if len(p.calls) != 2 {
		t.Errorf("fetched %d pages, want it to stop at 2", len(p.calls))
	}
}

func TestCollectDefaultLimitGuardsUnboundedCollection(t *testing.T) {
	// A server that always offers another page must not loop forever.
	calls := 0
	fetch := func(_ context.Context, opts ListOptions) (Page[string], error) {
		calls++
		return Page[string]{Data: []string{"x"}, NextCursor: fmt.Sprintf("cursor-%d", calls)}, nil
	}

	got, err := Collect(context.Background(), fetch, ListOptions{}, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != DefaultCollectLimit {
		t.Errorf("collected %d items, want the default cap of %d", len(got), DefaultCollectLimit)
	}
}

func TestCollectDetectsRepeatedCursor(t *testing.T) {
	fetch := func(_ context.Context, _ ListOptions) (Page[string], error) {
		return Page[string]{Data: []string{"x"}, NextCursor: "same"}, nil
	}

	got, err := Collect(context.Background(), fetch, ListOptions{}, 0)
	if err == nil {
		t.Fatal("Collect: want a loop error")
	}
	if !strings.Contains(err.Error(), "looping") {
		t.Errorf("error = %v, want it to report a loop", err)
	}
	if len(got) != 2 {
		t.Errorf("collected %d items, want the pages fetched before the loop was detected", len(got))
	}
}

func TestCollectDetectsEmptyPageWithCursor(t *testing.T) {
	fetch := func(_ context.Context, _ ListOptions) (Page[string], error) {
		return Page[string]{NextCursor: "next"}, nil
	}

	if _, err := Collect(context.Background(), fetch, ListOptions{}, 0); err == nil {
		t.Fatal("Collect: want an error for an empty page with a next cursor")
	}
}

func TestCollectReturnsPartialResultOnError(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	fetch := func(_ context.Context, _ ListOptions) (Page[string], error) {
		calls++
		if calls == 1 {
			return Page[string]{Data: []string{"a"}, NextCursor: "cursor-1"}, nil
		}
		return Page[string]{}, boom
	}

	got, err := Collect(context.Background(), fetch, ListOptions{}, 0)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want boom", err)
	}
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("Collect() = %v, want the page fetched before the failure", got)
	}
}

func TestCollectRejectsInvalidOptions(t *testing.T) {
	fetch := func(_ context.Context, _ ListOptions) (Page[string], error) {
		t.Error("fetch was called, want the options rejected first")
		return Page[string]{}, nil
	}

	if _, err := Collect(context.Background(), fetch, ListOptions{Limit: -1}, 0); err == nil {
		t.Fatal("Collect: want a validation error")
	}
}

func TestCollectHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	fetch := func(_ context.Context, _ ListOptions) (Page[string], error) {
		calls++
		cancel()
		return Page[string]{Data: []string{"a"}, NextCursor: fmt.Sprintf("cursor-%d", calls)}, nil
	}

	got, err := Collect(ctx, fetch, ListOptions{}, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(got) != 1 {
		t.Errorf("Collect() = %v, want the items collected before cancellation", got)
	}
}
