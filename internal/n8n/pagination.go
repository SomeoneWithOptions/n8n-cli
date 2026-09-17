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

// Collect walks every page and returns the accumulated items.
//
// max caps the number of items collected; zero uses [DefaultCollectLimit].
// Collection stops early on a repeated cursor, which is how a buggy or proxied
// endpoint presents an infinite loop.
func Collect[T any](ctx context.Context, fetch PageFunc[T], opts ListOptions, max int) ([]T, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if max <= 0 {
		max = DefaultCollectLimit
	}

	var items []T
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return items, err
		}

		page, err := fetch(ctx, opts)
		if err != nil {
			return items, err
		}
		items = append(items, page.Data...)

		if len(items) >= max {
			return items[:max], nil
		}
		if !page.HasMore() {
			return items, nil
		}
		if seen[page.NextCursor] {
			return items, fmt.Errorf("pagination cursor %q repeated: the API is looping", page.NextCursor)
		}
		if len(page.Data) == 0 {
			return items, errors.New("pagination returned an empty page with a next cursor: the API is looping")
		}
		seen[page.NextCursor] = true
		opts.Cursor = page.NextCursor
	}
}
