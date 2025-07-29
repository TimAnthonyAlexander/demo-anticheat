package stats

import (
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
)

// CheatDetector evaluates statistics to determine likelihood of cheating
type CheatDetector struct {
	*BaseCollector
}

// NewCheatDetector creates a new CheatDetector
func NewCheatDetector() *CheatDetector {
	return &CheatDetector{
		BaseCollector: NewBaseCollector("Cheat Detection", Category("anti_cheat")),
	}
}

// Setup does nothing for this collector as it only processes final stats
func (cd *CheatDetector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	// No setup needed
}

// CollectFrame does nothing for this collector as it only processes final stats
func (cd *CheatDetector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	// No per-frame processing needed
}

// CollectFinalStats calculates the cheating likelihood percentage for each player
func (cd *CheatDetector) CollectFinalStats(demoStats *DemoStats) {
	for _, playerStats := range demoStats.Players {
		// Calculate cheating likelihood based on available statistics
		likelihood := cd.calculateCheatLikelihood(playerStats)

		// Add the result as a metric
		playerStats.AddMetric(Category("anti_cheat"), Key("cheat_likelihood"), Metric{
			Type:        MetricPercentage,
			FloatValue:  likelihood,
			Description: "Estimated likelihood of player cheating",
		})
	}
}

// Helper function to clamp a value between 0 and 1
func clamp01(value float64) float64 {
	if value < 0.0 {
		return 0.0
	}
	if value > 1.0 {
		return 1.0
	}
	return value
}

// calculateCheatLikelihood determines the likelihood a player is cheating based on statistical analysis
func (cd *CheatDetector) calculateCheatLikelihood(playerStats *PlayerStats) float64 {
	// === Extract metrics ===
	hsPercentage := 0.0
	totalKills := int64(0)
	p95SnapVelocity := 0.0
	snapCount := int64(0)
	p10Reaction := 0.0
	reactionSamples := int64(0)

	if metric, found := playerStats.GetMetric(Category("kills"), Key("headshot_percentage")); found {
		hsPercentage = metric.FloatValue
	}

	if metric, found := playerStats.GetMetric(Category("kills"), Key("total_kills")); found {
		totalKills = metric.IntValue
	}

	if metric, found := playerStats.GetMetric(Category("aiming"), Key("p95_snap_velocity")); found {
		p95SnapVelocity = metric.FloatValue
	}

	if metric, found := playerStats.GetMetric(Category("aiming"), Key("snap_count")); found {
		snapCount = metric.IntValue
	}

	if metric, found := playerStats.GetMetric(Category("reaction"), Key("p10_reaction_time")); found {
		p10Reaction = metric.FloatValue
	}

	if metric, found := playerStats.GetMetric(Category("reaction"), Key("reaction_samples")); found {
		reactionSamples = metric.IntValue
	}

	// === Calculate cheat score using rule-based model ===

	// Headshot factor - only apply if player has at least 30 kills
	// 0 at 55%, 1 at 75%
	hsScore := 0.0
	if totalKills >= 30 {
		hsScore = clamp01((hsPercentage - 55.0) / 20.0)
	}

	// Snap velocity factor
	// 0 at 2°/ms, 1 at 3.5°/ms for sharper ramp
	snapScore := 0.0
	if snapCount >= 5 { // Need at least a few snaps for reliable data
		snapScore = clamp01((p95SnapVelocity - 2.0) / 1.5)
	}

	// Reaction time factor
	// 0 at 120ms, 1 at 60ms or below
	rtScore := 0.0
	if reactionSamples >= 5 { // Need at least a few samples for reliable data
		rtScore = clamp01((120.0 - p10Reaction) / 60.0)
	}

	// Recoil control factor
	// Score is calculated directly in the recoil collector
	// 0 at 0.75° mean error or higher, 1 at 0.30° mean error or lower
	recoilScore := 0.0
	if metric, found := playerStats.GetMetric(Category("recoil"), Key("recoil_score")); found {
		recoilScore = metric.FloatValue
	}

	// Calculate combined cheat score with adjusted weights as specified:
	// - 45% headshot score (reduced from 50%)
	// - 25% snap score (reduced from 30%)
	// - 15% reaction time score (reduced from 20%)
	// - 15% recoil control score (new component)
	cheatScore := 0.45*hsScore + 0.25*snapScore + 0.15*rtScore + 0.15*recoilScore

	// Flag as cheater if score >= 0.55 (55%)
	// Convert to percentage for reporting
	cheatLikelihood := cheatScore * 100.0

	// Apply additional rules based on game mode and kill count
	gameMode := "Competitive" // Default assumption
	roundCount := int64(30)   // Default assumption of regulation time (30 rounds)

	// Get game mode information
	if metric, found := playerStats.GetMetric(Category("game_info"), Key("game_mode")); found {
		gameMode = metric.StringValue
	}

	// Get round count information
	if metric, found := playerStats.GetMetric(Category("game_info"), Key("round_count")); found {
		roundCount = metric.IntValue
	}

	// Apply Wingman rule: For players with > 15 kills, increase likelihood by 20%
	if gameMode == "Wingman" && totalKills > 15 {
		cheatLikelihood = cheatLikelihood * 1.2

		// Add explanation
		playerStats.AddMetric(Category("anti_cheat"), Key("wingman_boost"), Metric{
			Type:        MetricString,
			StringValue: "Yes",
			Description: "Player has more than 15 kills in Wingman (20% boost applied)",
		})
	}

	// Apply Competitive rule: For players with > 39 kills in regulation (≤30 rounds), increase likelihood by 20%
	if gameMode == "Competitive" && totalKills > 39 && roundCount <= 30 {
		cheatLikelihood = cheatLikelihood * 1.2

		// Add explanation
		playerStats.AddMetric(Category("anti_cheat"), Key("competitive_boost"), Metric{
			Type:        MetricString,
			StringValue: "Yes",
			Description: "Player has more than 39 kills in regulation time (20% boost applied)",
		})
	}

	// Make sure we don't exceed 100%
	if cheatLikelihood > 100.0 {
		cheatLikelihood = 100.0
	}

	// === Add explanatory metrics for transparency ===
	playerStats.AddMetric(Category("anti_cheat"), Key("hs_score"), Metric{
		Type:        MetricFloat,
		FloatValue:  hsScore,
		Description: "Headshot-based cheat score component (0-1)",
	})

	playerStats.AddMetric(Category("anti_cheat"), Key("snap_score"), Metric{
		Type:        MetricFloat,
		FloatValue:  snapScore,
		Description: "Snap velocity-based cheat score component (0-1)",
	})

	playerStats.AddMetric(Category("anti_cheat"), Key("reaction_score"), Metric{
		Type:        MetricFloat,
		FloatValue:  rtScore,
		Description: "Reaction time-based cheat score component (0-1)",
	})

	playerStats.AddMetric(Category("anti_cheat"), Key("recoil_score"), Metric{
		Type:        MetricFloat,
		FloatValue:  recoilScore,
		Description: "Recoil control-based cheat score component (0-1)",
	})

	playerStats.AddMetric(Category("anti_cheat"), Key("total_cheat_score"), Metric{
		Type:        MetricFloat,
		FloatValue:  cheatScore,
		Description: "Total cheat score (0-1, ≥0.55 flags as cheater)",
	})

	// Mark as cheater if score exceeds threshold
	if cheatLikelihood >= 55.0 {
		playerStats.AddMetric(Category("anti_cheat"), Key("cheater"), Metric{
			Type:        MetricString,
			StringValue: "Yes",
			Description: "Player is flagged as a potential cheater",
		})
	} else {
		playerStats.AddMetric(Category("anti_cheat"), Key("cheater"), Metric{
			Type:        MetricString,
			StringValue: "No",
			Description: "Player is not flagged as a cheater",
		})
	}

	return cheatLikelihood
}
