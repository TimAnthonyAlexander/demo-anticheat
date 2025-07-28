package stats

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// Reporter defines the interface for statistics output formatters
type Reporter interface {
	// Report formats and outputs the statistics
	Report(demoStats *DemoStats, categories []Category, writer io.Writer) error
}

// TextReporter generates text-based reports for statistics
type TextReporter struct {
	title string
}

// NewTextReporter creates a new TextReporter
func NewTextReporter(title string) *TextReporter {
	return &TextReporter{title: title}
}

// Report generates a text-based report of the statistics
func (tr *TextReporter) Report(demoStats *DemoStats, categories []Category, writer io.Writer) error {
	if demoStats == nil || len(demoStats.Players) == 0 {
		_, err := fmt.Fprintln(writer, "No statistics available")
		return err
	}

	// Print title and demo information
	_, err := fmt.Fprintln(writer, tr.title)
	if err != nil {
		return err
	}

	if demoStats.DemoName != "" {
		_, err = fmt.Fprintf(writer, "Demo: %s\n", demoStats.DemoName)
		if err != nil {
			return err
		}
	}

	if demoStats.MapName != "" {
		_, err = fmt.Fprintf(writer, "Map: %s\n", demoStats.MapName)
		if err != nil {
			return err
		}
	}

	// Process each category
	for _, category := range categories {
		err = tr.reportCategory(demoStats, category, writer)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(writer)
		if err != nil {
			return err
		}
	}

	return nil
}

// reportCategory reports statistics for a specific category
func (tr *TextReporter) reportCategory(demoStats *DemoStats, category Category, writer io.Writer) error {
	// Get all stats keys for this category
	keys := make(map[Key]bool)
	for _, playerStats := range demoStats.Players {
		if categoryMap, exists := playerStats.Categories[category]; exists {
			for key := range categoryMap {
				keys[key] = true
			}
		}
	}

	// If no keys, this category has no data
	if len(keys) == 0 {
		return nil
	}

	// Title for the category
	_, err := fmt.Fprintf(writer, "\n=== %s Statistics ===\n\n", strings.Title(string(category)))
	if err != nil {
		return err
	}

	// Convert keys to slice for sorting
	keySlice := make([]Key, 0, len(keys))
	for key := range keys {
		keySlice = append(keySlice, key)
	}
	sort.Slice(keySlice, func(i, j int) bool {
		return string(keySlice[i]) < string(keySlice[j])
	})

	// Filter keys to display (exclude raw counts that are only used for calculations)
	displayKeys := make([]Key, 0)
	for _, key := range keySlice {
		// Only show percentage and other meaningful derived metrics, not raw counts
		if !strings.HasSuffix(string(key), "_ticks") {
			// If this is weapons category, ensure we show no_weapon_percentage
			if category == Category("weapons") && key == Key("no_weapon_percentage") {
				// Move this to the end
				continue
			}
			displayKeys = append(displayKeys, key)
		}
	}

	// For weapons category, add no_weapon_percentage at the end
	if category == Category("weapons") {
		noWeaponKey := Key("no_weapon_percentage")
		if keys[noWeaponKey] {
			displayKeys = append(displayKeys, noWeaponKey)
		}
	}

	if len(displayKeys) == 0 {
		return nil
	}

	// Get players and sort by name
	players := make([]*PlayerStats, 0, len(demoStats.Players))
	for _, playerStats := range demoStats.Players {
		players = append(players, playerStats)
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].Player.Name < players[j].Player.Name
	})

	// Set up tabwriter for aligned columns
	w := tabwriter.NewWriter(writer, 0, 0, 3, ' ', tabwriter.TabIndent)

	// Print header
	_, err = fmt.Fprint(w, "Player\tSteam ID")
	if err != nil {
		return err
	}

	for _, key := range displayKeys {
		_, err = fmt.Fprintf(w, "\t%s", formatColumnTitle(string(key)))
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(w)
	if err != nil {
		return err
	}

	// Print separator
	headerLength := 40 + (15 * len(displayKeys))
	_, err = fmt.Fprintln(w, strings.Repeat("-", headerLength))
	if err != nil {
		return err
	}

	// Print each player's stats
	for _, playerStats := range players {
		_, err = fmt.Fprintf(w, "%s\t%d", playerStats.Player.Name, playerStats.Player.SteamID64)
		if err != nil {
			return err
		}

		for _, key := range displayKeys {
			if metric, found := playerStats.GetMetric(category, key); found {
				_, err = fmt.Fprintf(w, "\t%s", formatMetricValue(metric))
				if err != nil {
					return err
				}
			} else {
				_, err = fmt.Fprint(w, "\t-")
				if err != nil {
					return err
				}
			}
		}
		_, err = fmt.Fprintln(w)
		if err != nil {
			return err
		}
	}

	return w.Flush()
}

// formatColumnTitle formats a key into a column title
func formatColumnTitle(key string) string {
	// Remove common suffixes
	key = strings.TrimSuffix(key, "_percentage")
	// Replace underscores with spaces and title case
	words := strings.Split(key, "_")
	for i, word := range words {
		words[i] = strings.Title(word)
	}
	return strings.Join(words, " ")
}

// formatMetricValue formats a metric for display
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
	default:
		return "-"
	}
}
