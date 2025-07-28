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

// calculateCheatLikelihood determines the likelihood a player is cheating based on statistical analysis
func (cd *CheatDetector) calculateCheatLikelihood(playerStats *PlayerStats) float64 {
	// Initialize factors and their weights
	type Factor struct {
		value  float64 // 0-100 score for this factor
		weight float64 // Weight of this factor in the overall calculation
	}

	factors := make(map[string]Factor)

	// === Headshot Analysis ===
	hsPercentage := 0.0
	totalKills := int64(0)

	if metric, found := playerStats.GetMetric(Category("kills"), Key("headshot_percentage")); found {
		hsPercentage = metric.FloatValue
	}

	if metric, found := playerStats.GetMetric(Category("kills"), Key("total_kills")); found {
		totalKills = metric.IntValue
	}

	// Weight headshot percentage by total kills (more kills = more reliable data)
	// 0-100 scale where 100 means extremely suspicious (e.g., 100% HS with many kills)
	hsFactorValue := 0.0
	if totalKills > 0 {
		// Base factor on headshot percentage
		hsFactorValue = hsPercentage

		// Apply multiplier based on kills (more kills with high HS% is more suspicious)
		if totalKills >= 10 {
			// For high kill counts, we increase the suspicion factor for high HS rates
			if hsPercentage > 70 {
				killsMultiplier := math.Min(1.5, 1.0+float64(totalKills)/100.0)
				hsFactorValue = math.Min(100.0, hsPercentage*killsMultiplier)
			}
		} else {
			// For low kill counts, we reduce the weight as it could just be luck
			hsFactorValue = hsPercentage * float64(totalKills) / 10.0
		}
	}
	factors["headshot"] = Factor{value: hsFactorValue, weight: 0.7} // Headshots are a strong indicator

	// === Weapon Usage Analysis ===
	knifePercentage := 0.0

	if metric, found := playerStats.GetMetric(Category("weapons"), Key("knife_percentage")); found {
		knifePercentage = metric.FloatValue
	}

	// Cheaters often have unusually low knife-out time (too confident moving around)
	// or sometimes unusually high (for trolling)
	knifeFactorValue := 0.0
	if knifePercentage < 10 {
		// Very low knife usage might indicate cheating (too confident moving without knife)
		knifeFactorValue = (10 - knifePercentage) * 5 // Up to 50 points for extremely low knife usage
	} else if knifePercentage > 40 {
		// Very high knife usage might also be suspicious (trolling with cheats)
		knifeFactorValue = (knifePercentage - 40) * 2 // Up to 60 points for extremely high knife usage
	}
	factors["knife_usage"] = Factor{value: knifeFactorValue, weight: 0.3}

	// === Calculate weighted average ===
	totalWeight := 0.0
	weightedSum := 0.0

	for _, factor := range factors {
		weightedSum += factor.value * factor.weight
		totalWeight += factor.weight
	}

	if totalWeight > 0 {
		return weightedSum / totalWeight
	}

	return 0.0
}
