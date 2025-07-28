package stats

import (
	"fmt"
	"io"
	"sort"
	"strings"
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
	fmt.Fprintln(writer, tr.title)

	if demoStats.DemoName != "" {
		fmt.Fprintf(writer, "Demo: %s\n", demoStats.DemoName)
	}

	if demoStats.MapName != "" {
		fmt.Fprintf(writer, "Map: %s\n", demoStats.MapName)
	}

	// Process each category
	for _, category := range categories {
		if err := tr.reportCategory(demoStats, category, writer); err != nil {
			return err
		}
		fmt.Fprintln(writer)
	}

	return nil
}

// reportCategory reports statistics for a specific category
func (tr *TextReporter) reportCategory(demoStats *DemoStats, category Category, writer io.Writer) error {
	// Get all metrics to display for this category
	displayKeys, hasData := tr.getDisplayKeys(demoStats, category)
	if !hasData {
		return nil
	}

	// Print category header
	fmt.Fprintf(writer, "\n=== %s Statistics ===\n\n", strings.Title(string(category)))

	// Get sorted players
	players := tr.getSortedPlayers(demoStats, category)

	// Column widths for formatting
	const (
		playerWidth  = 20
		steamIDWidth = 20
		valueWidth   = 12
		cheaterWidth = 7
	)

	// Print table header
	tr.printTableHeader(writer, category, displayKeys, playerWidth, steamIDWidth, valueWidth, cheaterWidth)

	// Print table rows
	for _, playerStats := range players {
		tr.printPlayerRow(writer, playerStats, category, displayKeys, playerWidth, steamIDWidth, valueWidth, cheaterWidth)
	}

	return nil
}

// getDisplayKeys returns the keys to display for a category and whether there's data to show
func (tr *TextReporter) getDisplayKeys(demoStats *DemoStats, category Category) ([]Key, bool) {
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
		return nil, false
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
		return nil, false
	}

	return displayKeys, true
}

// getSortedPlayers returns players sorted appropriately for the category
func (tr *TextReporter) getSortedPlayers(demoStats *DemoStats, category Category) []*PlayerStats {
	players := make([]*PlayerStats, 0, len(demoStats.Players))
	for _, playerStats := range demoStats.Players {
		players = append(players, playerStats)
	}

	// Sort players based on category
	if category == Category("anti_cheat") {
		// For anti-cheat category, sort by cheating likelihood (highest first)
		sort.Slice(players, func(i, j int) bool {
			iVal := getMetricFloatValue(players[i], category, Key("cheat_likelihood"))
			jVal := getMetricFloatValue(players[j], category, Key("cheat_likelihood"))
			return iVal > jVal
		})
	} else {
		// For other categories, sort by name
		sort.Slice(players, func(i, j int) bool {
			return players[i].Player.Name < players[j].Player.Name
		})
	}

	return players
}

// printTableHeader prints the header row for a table
func (tr *TextReporter) printTableHeader(writer io.Writer, category Category, displayKeys []Key, playerWidth, steamIDWidth, valueWidth, cheaterWidth int) {
	// Print header
	fmt.Fprintf(writer, "%-*s  %-*s  ", playerWidth, "Player", steamIDWidth, "Steam ID")

	for _, key := range displayKeys {
		fmt.Fprintf(writer, "%-*s  ", valueWidth, formatColumnTitle(string(key)))
	}

	// For anti-cheat, add a "Cheater" column
	if category == Category("anti_cheat") {
		fmt.Fprintf(writer, "%-*s", cheaterWidth, "Cheater")
	}

	fmt.Fprintln(writer)

	// Print separator
	headerLength := playerWidth + steamIDWidth + (valueWidth * len(displayKeys)) + (len(displayKeys) * 2) + 4
	if category == Category("anti_cheat") {
		headerLength += cheaterWidth + 2 // Extra for the "Cheater" column
	}
	fmt.Fprintln(writer, strings.Repeat("-", headerLength))
}

// printPlayerRow prints a row for a player in a table
func (tr *TextReporter) printPlayerRow(writer io.Writer, playerStats *PlayerStats, category Category, displayKeys []Key, playerWidth, steamIDWidth, valueWidth, cheaterWidth int) {
	// Print player name and ID
	fmt.Fprintf(writer, "%-*s  %-*d  ", playerWidth, playerStats.Player.Name, steamIDWidth, playerStats.Player.SteamID64)

	// Used to track cheat likelihood for the "Cheater" column
	cheatLikelihood := 0.0

	// Print each metric value
	for _, key := range displayKeys {
		if metric, found := playerStats.GetMetric(category, key); found {
			fmt.Fprintf(writer, "%-*s  ", valueWidth, formatMetricValue(metric))

			// Store cheat likelihood for later
			if category == Category("anti_cheat") && key == Key("cheat_likelihood") {
				cheatLikelihood = metric.FloatValue
			}
		} else {
			fmt.Fprintf(writer, "%-*s  ", valueWidth, "-")
		}
	}

	// Add "Yes/No" column for anti-cheat category
	if category == Category("anti_cheat") {
		if cheatLikelihood >= 90.0 {
			fmt.Fprintf(writer, "%-*s", cheaterWidth, "Yes")
		} else {
			fmt.Fprintf(writer, "%-*s", cheaterWidth, "No")
		}
	}

	fmt.Fprintln(writer)
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

// getMetricFloatValue is a helper to safely get a float value from a metric
func getMetricFloatValue(playerStats *PlayerStats, category Category, key Key) float64 {
	if metric, found := playerStats.GetMetric(category, key); found {
		return metric.FloatValue
	}
	return 0.0
}
