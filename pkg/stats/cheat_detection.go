package stats

import (
	"math"

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
	medianSnapVelocity := 0.0
	snapCount := int64(0)

	if metric, found := playerStats.GetMetric(Category("kills"), Key("headshot_percentage")); found {
		hsPercentage = metric.FloatValue
	}

	if metric, found := playerStats.GetMetric(Category("kills"), Key("total_kills")); found {
		totalKills = metric.IntValue
	}

	if metric, found := playerStats.GetMetric(Category("aiming"), Key("p95_snap_velocity")); found {
		p95SnapVelocity = metric.FloatValue
	}

	if metric, found := playerStats.GetMetric(Category("aiming"), Key("median_snap_velocity")); found {
		medianSnapVelocity = metric.FloatValue
	}

	if metric, found := playerStats.GetMetric(Category("aiming"), Key("snap_count")); found {
		snapCount = metric.IntValue
	}

	// === Calculate cheat score using rule-based model ===
	score := 0.0

	// Headshot factor - only apply if player has at least 30 kills
	// HS above 55% adds up to 1.0 to the score
	hsScore := 0.0
	if totalKills >= 30 {
		hsScore = clamp01((hsPercentage - 55.0) / 20.0)
		score += 1.0 * hsScore
	}

	// Snap velocity factor
	// P95 snap above 3°/ms adds up to 1.0 to the score
	snapScore := 0.0
	if snapCount >= 5 {
		snapScore = clamp01((p95SnapVelocity - 3.0) / 3.0)
		score += 1.0 * snapScore

		// Also consider consistency - if median is close to p95, it's more suspicious
		consistencyFactor := medianSnapVelocity / math.Max(0.001, p95SnapVelocity)
		if consistencyFactor > 0.7 && snapScore > 0.5 {
			score += 0.2 * consistencyFactor // Add up to 0.2 for high consistency
		}
	}

	// Flag player as potential cheater if total score exceeds threshold
	// According to the guidelines, a score >= 1.2 is considered suspicious
	// This means the player is red on both metrics or extreme on one
	cheatLikelihood := 0.0
	if score >= 1.2 {
		// Scale to percentage - at 1.2 we're at 60%, anything above 2.0 is 100%
		cheatLikelihood = math.Min(100.0, (score/2.0)*100.0)
	} else {
		// Below the cheating threshold, scale proportionally
		cheatLikelihood = (score / 1.2) * 60.0
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

	playerStats.AddMetric(Category("anti_cheat"), Key("total_cheat_score"), Metric{
		Type:        MetricFloat,
		FloatValue:  score,
		Description: "Total cheat score before conversion to percentage",
	})

	return cheatLikelihood
}
