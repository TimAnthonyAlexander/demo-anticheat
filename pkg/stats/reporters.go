package stats

import (
	"fmt"
	"io"
)

// Reporter defines the interface for statistics output formatters.
type Reporter interface {
	Report(demoStats *DemoStats, categories []Category, writer io.Writer) error
}

// TextReporter renders the colored, layout-rich terminal report. The
// rendering logic lives in term_renderer.go and is auto-degraded to plain
// ASCII when the writer is not a TTY or NO_COLOR is set.
type TextReporter struct {
	title string
}

// NewTextReporter creates a TextReporter that prints `title` in the header.
func NewTextReporter(title string) *TextReporter {
	return &TextReporter{title: title}
}

// Report renders the report. The categories argument is accepted for
// Reporter compatibility but is unused — the renderer derives its own
// ordering from html_reporter.go's shared builders.
func (tr *TextReporter) Report(demoStats *DemoStats, _ []Category, writer io.Writer) error {
	return renderTerminal(demoStats, writer, tr.title)
}

// formatMetricValue formats a metric for display. Shared with the HTML
// reporter and the category-block renderer.
func formatMetricValue(metric Metric) string {
	switch metric.Type {
	case MetricPercentage:
		return fmt.Sprintf("%.2f%%", metric.FloatValue)
	case MetricFloat:
		return fmt.Sprintf("%.2f", metric.FloatValue)
	case MetricInteger, MetricCount:
		return fmt.Sprintf("%d", metric.IntValue)
	case MetricDuration:
		return metric.DurationValue.String()
	case MetricString:
		if metric.StringValue == "" {
			return "-"
		}
		return metric.StringValue
	default:
		return "-"
	}
}

// getMetricFloatValue safely returns the FloatValue of a metric or 0.
func getMetricFloatValue(playerStats *PlayerStats, category Category, key Key) float64 {
	if metric, found := playerStats.GetMetric(category, key); found {
		return metric.FloatValue
	}
	return 0.0
}

