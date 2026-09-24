package cli

import (
	"context"
	"slices"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// executionMultiStatusPageSize is the merged page size when several statuses
// are listed without --limit. It is smaller than the server default of 100 so
// a merged list stays readable, and it is sent explicitly on every per-status
// request so the merge knows how many rows each list contributed.
const executionMultiStatusPageSize = 30

// listExecutionsForStatuses fetches each status separately, because
// GET /executions filters on one status per request, and merges the results
// newest first. Requests are sequential and follow the order the statuses
// were given; the first error ends the listing with no partial result.
//
// Without all, each status contributes its first page of opts.Limit rows
// (executionMultiStatusPageSize when unset) and the merge keeps the newest
// opts.Limit. Every per-status list is newest first, so the newest N of the
// union always lie within the union of each list's newest N: the result is
// exactly the N newest executions matching any of the statuses. With all,
// each status is walked with its own cursor and the merge is capped at
// [n8n.DefaultCollectLimit] by the same argument.
//
// more reports, in page mode, that at least one status had further pages or
// the merge dropped rows, so the caller can say the list is not exhaustive.
// The returned page never carries a cursor: each status pages with its own.
func listExecutionsForStatuses(ctx context.Context, client *n8n.Client, opts n8n.ListExecutionsOptions, statuses []string, all bool) (n8n.Page[n8n.Execution], bool, error) {
	if !all && opts.Limit == 0 {
		opts.Limit = executionMultiStatusPageSize
	}
	lists := make([][]n8n.Execution, 0, len(statuses))
	more := false
	for _, status := range statuses {
		statusOpts := opts
		statusOpts.Status = status
		if all {
			fetch := func(ctx context.Context, pageOpts n8n.ListOptions) (n8n.Page[n8n.Execution], error) {
				next := statusOpts
				next.ListOptions = pageOpts
				return client.ListExecutions(ctx, next)
			}
			data, err := n8n.Collect(ctx, fetch, statusOpts.ListOptions, n8n.DefaultCollectLimit)
			if err != nil {
				return n8n.Page[n8n.Execution]{}, false, err
			}
			lists = append(lists, data)
			continue
		}
		page, err := client.ListExecutions(ctx, statusOpts)
		if err != nil {
			return n8n.Page[n8n.Execution]{}, false, err
		}
		more = more || page.HasMore()
		lists = append(lists, page.Data)
	}
	limit := opts.Limit
	if all {
		limit = n8n.DefaultCollectLimit
	}
	merged, truncated := mergeExecutionsNewestFirst(limit, lists...)
	return n8n.Page[n8n.Execution]{Data: merged}, !all && (more || truncated), nil
}

// mergeExecutionsNewestFirst concatenates lists in order, keeps the first
// occurrence of each ID, sorts by descending ID and truncates to max when max
// is positive. An execution can change status between two requests (running
// then success) and so appear in two lists; keeping the first occurrence lets
// the caller decide which list wins by its order. The result is never nil.
func mergeExecutionsNewestFirst(max int, lists ...[]n8n.Execution) (merged []n8n.Execution, truncated bool) {
	seen := map[string]bool{}
	merged = []n8n.Execution{}
	for _, list := range lists {
		for _, execution := range list {
			if !seen[execution.ID] {
				seen[execution.ID] = true
				merged = append(merged, execution)
			}
		}
	}
	slices.SortStableFunc(merged, func(a, b n8n.Execution) int { return compareExecutionIDs(b.ID, a.ID) })
	if max > 0 && len(merged) > max {
		return merged[:max], true
	}
	return merged, false
}
