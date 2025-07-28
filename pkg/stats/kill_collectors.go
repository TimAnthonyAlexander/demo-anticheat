package stats

import (
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

// HeadshotCollector tracks headshot kill statistics
type HeadshotCollector struct {
	*BaseCollector
}

// NewHeadshotCollector creates a new HeadshotCollector
func NewHeadshotCollector() *HeadshotCollector {
	return &HeadshotCollector{
		BaseCollector: NewBaseCollector("Headshot Statistics", Category("kills")),
	}
}

// Setup registers event handlers for kill events
func (hc *HeadshotCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	// Register kill event handler
	parser.RegisterEventHandler(func(e events.Kill) {
		// Ignore suicides and team kills
		if e.Killer == nil || e.Victim == nil || e.Killer == e.Victim || e.Killer.Team == e.Victim.Team {
			return
		}

		// Get player stats for the killer
		playerStats := demoStats.GetOrCreatePlayerStats(e.Killer)
		if playerStats == nil {
			return
		}

		// Increment total kills
		playerStats.IncrementIntMetric(Category("kills"), Key("total_kills"))

		// Increment headshot kills if applicable
		if e.IsHeadshot {
			playerStats.IncrementIntMetric(Category("kills"), Key("headshot_kills"))
		}
	})
}

// CollectFrame is not needed for this collector as we're using event handlers
func (hc *HeadshotCollector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	// No per-frame processing needed, we use event handlers
}

// CollectFinalStats calculates headshot percentage
func (hc *HeadshotCollector) CollectFinalStats(demoStats *DemoStats) {
	for _, playerStats := range demoStats.Players {
		totalKills, found := playerStats.GetMetric(Category("kills"), Key("total_kills"))
		if !found || totalKills.IntValue == 0 {
			continue
		}

		// Calculate headshot percentage
		if hsKills, found := playerStats.GetMetric(Category("kills"), Key("headshot_kills")); found {
			hsPercentage := float64(hsKills.IntValue) / float64(totalKills.IntValue) * 100
			playerStats.AddMetric(Category("kills"), Key("headshot_percentage"), Metric{
				Type:        MetricPercentage,
				FloatValue:  hsPercentage,
				Description: "Percentage of kills that were headshots",
			})
		} else {
			// If player has kills but no HS kills, set to 0%
			playerStats.AddMetric(Category("kills"), Key("headshot_percentage"), Metric{
				Type:        MetricPercentage,
				FloatValue:  0,
				Description: "Percentage of kills that were headshots",
			})
		}
	}
}
