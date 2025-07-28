package stats

import (
	"time"

	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
)

// PlayerIdentifier contains information to identify a player
type PlayerIdentifier struct {
	SteamID64 uint64
	Name      string
}

// Category represents a category of statistics (e.g., weapons, movement, etc.)
type Category string

// Key represents a specific statistic key within a category
type Key string

// MetricType defines the type of a statistic value (count, percentage, duration, etc.)
type MetricType string

const (
	// MetricCount represents a count metric
	MetricCount MetricType = "count"
	// MetricPercentage represents a percentage metric
	MetricPercentage MetricType = "percentage"
	// MetricDuration represents a duration metric
	MetricDuration MetricType = "duration"
	// MetricFloat represents a floating point value
	MetricFloat MetricType = "float"
	// MetricInteger represents an integer value
	MetricInteger MetricType = "integer"
	// MetricString represents a string value
	MetricString MetricType = "string"
)

// Metric represents a single statistical measure
type Metric struct {
	Type          MetricType
	FloatValue    float64
	IntValue      int64
	DurationValue time.Duration
	StringValue   string
	Description   string
}

// PlayerStats contains all statistics for a player
type PlayerStats struct {
	Player     PlayerIdentifier
	Categories map[Category]map[Key]Metric
}

// NewPlayerStats creates a new PlayerStats instance
func NewPlayerStats(player *common.Player) *PlayerStats {
	return &PlayerStats{
		Player: PlayerIdentifier{
			SteamID64: player.SteamID64,
			Name:      player.Name,
		},
		Categories: make(map[Category]map[Key]Metric),
	}
}

// AddMetric adds or updates a metric for a player
func (ps *PlayerStats) AddMetric(category Category, key Key, metric Metric) {
	if _, exists := ps.Categories[category]; !exists {
		ps.Categories[category] = make(map[Key]Metric)
	}
	ps.Categories[category][key] = metric
}

// GetMetric retrieves a metric for a player
func (ps *PlayerStats) GetMetric(category Category, key Key) (Metric, bool) {
	if categoryMap, exists := ps.Categories[category]; exists {
		if metric, found := categoryMap[key]; found {
			return metric, true
		}
	}
	return Metric{}, false
}

// IncrementIntMetric increments an integer metric
func (ps *PlayerStats) IncrementIntMetric(category Category, key Key) {
	if _, exists := ps.Categories[category]; !exists {
		ps.Categories[category] = make(map[Key]Metric)
		ps.Categories[category][key] = Metric{
			Type:     MetricInteger,
			IntValue: 1,
		}
		return
	}

	if metric, found := ps.Categories[category][key]; found {
		metric.IntValue++
		ps.Categories[category][key] = metric
	} else {
		ps.Categories[category][key] = Metric{
			Type:     MetricInteger,
			IntValue: 1,
		}
	}
}

// IncrementFloatMetric adds a value to a float metric
func (ps *PlayerStats) IncrementFloatMetric(category Category, key Key, value float64) {
	if _, exists := ps.Categories[category]; !exists {
		ps.Categories[category] = make(map[Key]Metric)
		ps.Categories[category][key] = Metric{
			Type:       MetricFloat,
			FloatValue: value,
		}
		return
	}

	if metric, found := ps.Categories[category][key]; found {
		metric.FloatValue += value
		ps.Categories[category][key] = metric
	} else {
		ps.Categories[category][key] = Metric{
			Type:       MetricFloat,
			FloatValue: value,
		}
	}
}

// DemoStats contains statistics for all players in a demo
type DemoStats struct {
	Players   map[uint64]*PlayerStats
	TickRate  float64
	TickCount int
	DemoName  string
	MapName   string
}

// NewDemoStats creates a new DemoStats instance
func NewDemoStats() *DemoStats {
	return &DemoStats{
		Players: make(map[uint64]*PlayerStats),
	}
}

// GetOrCreatePlayerStats gets existing player stats or creates new ones if they don't exist
func (ds *DemoStats) GetOrCreatePlayerStats(player *common.Player) *PlayerStats {
	if player == nil {
		return nil
	}

	if _, exists := ds.Players[player.SteamID64]; !exists {
		ds.Players[player.SteamID64] = NewPlayerStats(player)
	}
	return ds.Players[player.SteamID64]
}

// GetOrCreatePlayerStatsBySteamID gets existing player stats or creates new ones by SteamID
func (ds *DemoStats) GetOrCreatePlayerStatsBySteamID(steamID uint64) *PlayerStats {
	if _, exists := ds.Players[steamID]; !exists {
		// Create a placeholder player
		ds.Players[steamID] = &PlayerStats{
			Player: PlayerIdentifier{
				SteamID64: steamID,
				Name:      "Unknown",
			},
			Categories: make(map[Category]map[Key]Metric),
		}
	}
	return ds.Players[steamID]
}
