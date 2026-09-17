package n8n

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// InsightUnit describes how one insight metric value is measured.
type InsightUnit string

const (
	InsightUnitCount       InsightUnit = "count"
	InsightUnitRatio       InsightUnit = "ratio"
	InsightUnitMinute      InsightUnit = "minute"
	InsightUnitMillisecond InsightUnit = "millisecond"
)

// InsightMetric is one aggregate value and its change from the preceding
// period. Deviation is nil when the API has no preceding-period comparison.
type InsightMetric struct {
	Value     float64     `json:"value"`
	Deviation *float64    `json:"deviation"`
	Unit      InsightUnit `json:"unit"`
}

// InsightsSummary is the aggregate execution summary for a selected date
// range and optional project.
type InsightsSummary struct {
	Total          InsightMetric `json:"total"`
	Failed         InsightMetric `json:"failed"`
	FailureRate    InsightMetric `json:"failureRate"`
	TimeSaved      InsightMetric `json:"timeSaved"`
	AverageRunTime InsightMetric `json:"averageRunTime"`
}

// GetInsightsSummaryOptions selects the period and optional project used to
// calculate an insights summary. Empty dates use the server defaults: seven
// days ago for StartDate and now for EndDate.
type GetInsightsSummaryOptions struct {
	StartDate string
	EndDate   string
	ProjectID string
}

// Validate rejects filters outside the endpoint contract before transport.
func (o GetInsightsSummaryOptions) Validate() error {
	if err := validateInsightTimeRange(o.StartDate, o.EndDate); err != nil {
		return err
	}
	if strings.TrimSpace(o.ProjectID) != o.ProjectID {
		return fmt.Errorf("project ID must not start or end with whitespace")
	}
	return nil
}

func (o GetInsightsSummaryOptions) apply() url.Values {
	query := url.Values{}
	if o.StartDate != "" {
		query.Set("startDate", o.StartDate)
	}
	if o.EndDate != "" {
		query.Set("endDate", o.EndDate)
	}
	if o.ProjectID != "" {
		query.Set("projectId", o.ProjectID)
	}
	return query
}

// GetInsightsSummary returns aggregate execution insights for the selected
// period and project scope.
func (c *Client) GetInsightsSummary(ctx context.Context, opts GetInsightsSummaryOptions) (*InsightsSummary, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	var summary InsightsSummary
	if _, err := c.Do(ctx, Request{
		Path:  PathJoin("insights", "summary"),
		Query: opts.apply(),
	}, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func validateInsightTimeRange(startDate, endDate string) error {
	parse := func(name, value string) (time.Time, error) {
		if value == "" {
			return time.Time{}, nil
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s must be RFC3339, for example 2026-09-17T14:30:00Z: %w", name, err)
		}
		return parsed, nil
	}
	start, err := parse("start-date", startDate)
	if err != nil {
		return err
	}
	end, err := parse("end-date", endDate)
	if err != nil {
		return err
	}
	if !start.IsZero() && !end.IsZero() && start.After(end) {
		return fmt.Errorf("start-date must not be later than end-date")
	}
	return nil
}
