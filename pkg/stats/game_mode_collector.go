package stats

import (
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

// GameModeCollector tracks information about the game mode and round counts
type GameModeCollector struct {
	*BaseCollector
	roundCount int
}

// NewGameModeCollector creates a new GameModeCollector
func NewGameModeCollector() *GameModeCollector {
	return &GameModeCollector{
		BaseCollector: NewBaseCollector("Game Mode", Category("game_info")),
		roundCount:    0,
	}
}

// Setup registers event handlers for round events
func (gmc *GameModeCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	// Track round end events to count rounds
	parser.RegisterEventHandler(func(e events.RoundEnd) {
		gmc.roundCount++
	})
}

// CollectFrame is not needed for this collector as we're using event handlers
func (gmc *GameModeCollector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	// No per-frame processing needed, we use event handlers
}

// CollectFinalStats calculates game mode and stores round count
func (gmc *GameModeCollector) CollectFinalStats(demoStats *DemoStats) {
	// Create a general game info metric for the demo
	gameInfoMetric := Metric{
		Type:        MetricInteger,
		IntValue:    int64(gmc.roundCount),
		Description: "Number of rounds played",
	}

	// Since DemoStats doesn't have an AddMetric method, we'll store this in a "global" player stats
	// Create a special "global" player to store demo-wide metrics if it doesn't exist
	globalStats := demoStats.GetOrCreatePlayerStatsBySteamID(0)
	globalStats.AddMetric(Category("game_info"), Key("round_count"), gameInfoMetric)

	// Determine game mode based on player count
	playerCount := len(demoStats.Players)

	// Game mode detection is approximate:
	// - Wingman typically has 4 or fewer players
	// - Competitive typically has 8-10 players
	isWingman := playerCount <= 4

	// Store game mode
	if isWingman {
		globalStats.AddMetric(Category("game_info"), Key("game_mode"), Metric{
			Type:        MetricString,
			StringValue: "Wingman",
			Description: "Detected game mode",
		})
	} else {
		globalStats.AddMetric(Category("game_info"), Key("game_mode"), Metric{
			Type:        MetricString,
			StringValue: "Competitive",
			Description: "Detected game mode",
		})
	}

	// Also store the game mode and round count for each player for easier access
	for _, playerStats := range demoStats.Players {
		playerStats.AddMetric(Category("game_info"), Key("round_count"), gameInfoMetric)

		if isWingman {
			playerStats.AddMetric(Category("game_info"), Key("game_mode"), Metric{
				Type:        MetricString,
				StringValue: "Wingman",
				Description: "Detected game mode",
			})
		} else {
			playerStats.AddMetric(Category("game_info"), Key("game_mode"), Metric{
				Type:        MetricString,
				StringValue: "Competitive",
				Description: "Detected game mode",
			})
		}
	}
}
