package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// collectionInfo is the "collection" object added to the JSON of a bounded
// collection (--all, or a merged multi-status page), so a script can tell an
// exhausted list from one that stopped at the bound without reading stderr.
type collectionInfo struct {
	// Truncated means the walk stopped without proving the list exhausted; it
	// does not mean the number of remaining items is known.
	Truncated bool `json:"truncated"`
	// Limit is the bound of the collected result, not the API page size.
	Limit int `json:"limit"`
	// NextSkip is the offset resuming an incomplete offset-paged collection.
	NextSkip *int `json:"nextSkip,omitempty"`
}

// collectedPage is the JSON of a bounded cursor collection: the API page shape
// plus the collection report. NextCursor is set only when resuming from it
// cannot skip a discarded item.
type collectedPage[T any] struct {
	Data       []T            `json:"data"`
	NextCursor string         `json:"nextCursor"`
	Collection collectionInfo `json:"collection"`
}

// collectedFolderPage is the JSON of a bounded folder collection: count stays
// the server's matching total, which can exceed the folders shown.
type collectedFolderPage struct {
	Count      int            `json:"count"`
	Data       []n8n.Folder   `json:"data"`
	Collection collectionInfo `json:"collection"`
}

// Resume guidance kinds for an incomplete collection.
const (
	resumeCursor      = "cursor"
	resumeClipped     = "clipped"
	resumeSkip        = "skip"
	resumeMultiStatus = "multi-status"
	// resumeMultiStatusPage is a merged multi-status page without --all.
	resumeMultiStatusPage = "multi-status-page"
)

// listCollection describes how a bounded collection ended. A nil
// *listCollection means the command printed one ordinary API page.
type listCollection struct {
	info   collectionInfo
	resume string
	cursor string
	skip   int
}

// collectPage walks every page with fetch, up to [n8n.DefaultCollectLimit],
// and returns the items as a page carrying the resumable cursor, if any.
func collectPage[T any](ctx context.Context, fetch n8n.PageFunc[T], opts n8n.ListOptions) (n8n.Page[T], *listCollection, error) {
	collection, err := n8n.CollectWithInfo(ctx, fetch, opts, n8n.DefaultCollectLimit)
	if err != nil {
		return n8n.Page[T]{}, nil, err
	}
	page := n8n.Page[T]{Data: collection.Data, NextCursor: collection.NextCursor}
	return page, cursorCollection(collection.Truncated, collection.NextCursor, n8n.DefaultCollectLimit), nil
}

// cursorCollection reports a cursor collection bounded at limit.
func cursorCollection(truncated bool, cursor string, limit int) *listCollection {
	c := &listCollection{info: collectionInfo{Truncated: truncated, Limit: limit}, cursor: cursor}
	if cursor != "" {
		c.resume = resumeCursor
	} else {
		c.resume = resumeClipped
	}
	return c
}

// collectFolderPage walks every folder page, up to [n8n.DefaultCollectLimit],
// keeping the server's matching count.
func collectFolderPage(ctx context.Context, fetch n8n.FolderPageFunc, opts n8n.ListFoldersOptions) (n8n.FolderPage, *listCollection, error) {
	collection, err := n8n.CollectFoldersWithInfo(ctx, fetch, opts, n8n.DefaultCollectLimit)
	if err != nil {
		return n8n.FolderPage{}, nil, err
	}
	c := &listCollection{
		info:   collectionInfo{Truncated: collection.Truncated, Limit: n8n.DefaultCollectLimit},
		resume: resumeSkip,
		skip:   collection.NextSkip,
	}
	if collection.Truncated {
		c.info.NextSkip = &c.skip
	}
	return n8n.FolderPage{Count: collection.Count, Data: collection.Data}, c, nil
}

// writePageJSON writes page as the API shape, adding the collection report
// when the page is a bounded collection.
func writePageJSON[T any](w io.Writer, page n8n.Page[T], c *listCollection) error {
	if c == nil {
		return writeJSON(w, page)
	}
	data := page.Data
	if data == nil {
		data = []T{}
	}
	return writeJSON(w, collectedPage[T]{Data: data, NextCursor: page.NextCursor, Collection: c.info})
}

// writeFolderPageJSON is [writePageJSON] for the offset-paged folder list.
func writeFolderPageJSON(w io.Writer, page n8n.FolderPage, c *listCollection) error {
	if c == nil {
		return writeJSON(w, page)
	}
	data := page.Data
	if data == nil {
		data = []n8n.Folder{}
	}
	return writeJSON(w, collectedFolderPage{Count: page.Count, Data: data, Collection: c.info})
}

// finishCollection warns on stderr, after stdout was written successfully,
// when the collection is incomplete. It returns renderErr unchanged.
func finishCollection(w io.Writer, c *listCollection, renderErr error) error {
	if renderErr != nil || c == nil || !c.info.Truncated {
		return renderErr
	}
	if c.resume == resumeMultiStatusPage {
		fmt.Fprintln(w, "More executions match these statuses; use --all (up to 10,000) or a single --status with --cursor to page further.")
		return nil
	}
	fmt.Fprintf(w, "Warning: collection stopped at the %d-item limit; additional matches may remain.\n", c.info.Limit)
	switch c.resume {
	case resumeCursor:
		// The cursor is server data: escape it like every other cursor hint so
		// a control character cannot break the line or drive the terminal.
		fmt.Fprintf(w, "Continue with --cursor %s, keeping the same context, URL, filters and --limit.\n", escapeScriptField(c.cursor))
	case resumeClipped:
		fmt.Fprintln(w, "The limit fell inside a page, so no cursor resumes without skipping items; narrow the filters, or page explicitly with --limit and --cursor.")
	case resumeSkip:
		fmt.Fprintf(w, "Continue with --skip %d, keeping the same project, filters, --sort-by and --select.\n", c.skip)
	case resumeMultiStatus:
		fmt.Fprintln(w, "Repeating --all does not reach the omitted executions; narrow the workflow, project or time filters, or list each status separately with --cursor.")
	}
	return nil
}

// cursorCollectionHelp closes the Long help of every cursor list with --all.
const cursorCollectionHelp = "With --all, JSON output adds a collection object. Its truncated field is true,\n" +
	"and a warning goes to stderr, when the walk stopped at the 10,000-item limit\n" +
	"without reaching the end. nextCursor then resumes the walk when the limit fell\n" +
	"between pages; it stays empty when the limit fell inside a page, because\n" +
	"resuming from the next page would skip items: narrow the filters or page\n" +
	"explicitly with --limit and --cursor instead."

// folderCollectionHelp closes the Long help of the offset-paged folder list.
const folderCollectionHelp = "With --all, count stays the server's matching total, including folders before\n" +
	"--skip, so it can exceed the folders shown. JSON output adds a collection\n" +
	"object: when the walk stopped at the 10,000-folder limit without reaching the\n" +
	"end, truncated is true, nextSkip is the --skip that continues it, and a warning\n" +
	"goes to stderr."
