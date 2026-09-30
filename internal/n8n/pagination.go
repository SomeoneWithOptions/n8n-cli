package n8n

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// DefaultCollectLimit caps [Collect] when the caller sets no maximum, so an
// --all flag on a large instance cannot fill memory or loop forever.
const DefaultCollectLimit = 10_000

// ListOptions are the cursor pagination parameters shared by list operations.
type ListOptions struct {
	// Limit is the page size requested from the API. Zero uses the server default.
	Limit int
	// Cursor is the nextCursor of a previous page. Empty starts at the first page.
	Cursor string
}

// Apply writes the options into q, creating it when nil, and returns it.
func (o ListOptions) Apply(q url.Values) url.Values {
	if q == nil {
		q = url.Values{}
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.Cursor != "" {
		q.Set("cursor", o.Cursor)
	}
	return q
}

// Validate rejects options the API would reject or that indicate a bug.
func (o ListOptions) Validate() error {
	if o.Limit < 0 {
		return fmt.Errorf("limit must not be negative, got %d", o.Limit)
	}
	return nil
}

// Page is one cursor-paginated response. NextCursor is empty on the last page.
type Page[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"nextCursor"`
}

// HasMore reports whether another page is available.
func (p Page[T]) HasMore() bool { return p.NextCursor != "" }

// PageFunc fetches one page for the given options.
type PageFunc[T any] func(context.Context, ListOptions) (Page[T], error)

// Collection is the result of walking a cursor-paginated list up to a bound.
type Collection[T any] struct {
	// Data is the collected items, never nil.
	Data []T
	// NextCursor resumes the walk exactly after Data. It is set only when the
	// bound fell on a whole-page boundary and the server offered another page;
	// a bound that cuts through a page leaves it empty, because a server cursor
	// cannot point inside a page and resuming from the next one would skip the
	// discarded items.
	NextCursor string
	// Truncated reports that the walk stopped at the bound without proving the
	// list was exhausted. It is meaningful only when no error was returned.
	Truncated bool
}

// Collect walks every page and returns the accumulated items. It is
// [CollectWithInfo] without the completeness report, for callers that only
// need the items.
func Collect[T any](ctx context.Context, fetch PageFunc[T], opts ListOptions, max int) ([]T, error) {
	collection, err := CollectWithInfo(ctx, fetch, opts, max)
	return collection.Data, err
}

// CollectWithInfo walks every page, starting at opts.Cursor with opts.Limit
// items per page, and reports whether the list was exhausted.
//
// max caps the number of items collected; zero uses [DefaultCollectLimit].
// Reaching the cap sends no extra request to probe for more: a page that fits
// exactly and carries no next cursor is complete, one that carries a next
// cursor is truncated and resumable, and one that has to be clipped is
// truncated without a cursor. A repeated cursor, including the starting one,
// or an empty page with a next cursor is how a buggy or proxied endpoint
// presents an infinite loop, and is an error.
//
// On error the items fetched so far are returned with it; they are not a
// complete collection.
func CollectWithInfo[T any](ctx context.Context, fetch PageFunc[T], opts ListOptions, max int) (Collection[T], error) {
	if err := opts.Validate(); err != nil {
		return Collection[T]{}, err
	}
	if max <= 0 {
		max = DefaultCollectLimit
	}

	result := Collection[T]{Data: []T{}}
	seen := map[string]bool{}
	if opts.Cursor != "" {
		seen[opts.Cursor] = true
	}
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		page, err := fetch(ctx, opts)
		if err != nil {
			return result, err
		}
		kept := min(len(page.Data), max-len(result.Data))
		result.Data = append(result.Data, page.Data[:kept]...)

		if page.HasMore() {
			if seen[page.NextCursor] {
				return result, fmt.Errorf("pagination cursor %q repeated: the API is looping", page.NextCursor)
			}
			if len(page.Data) == 0 {
				return result, errors.New("pagination returned an empty page with a next cursor: the API is looping")
			}
		}
		if kept < len(page.Data) {
			result.Truncated = true
			return result, nil
		}
		if !page.HasMore() {
			return result, nil
		}
		if len(result.Data) >= max {
			result.Truncated = true
			result.NextCursor = page.NextCursor
			return result, nil
		}
		seen[page.NextCursor] = true
		opts.Cursor = page.NextCursor
	}
}
