package stats

import "fmt"

// cheatscoreFlagThreshold is the cheat_likelihood at or above which a player
// is flagged. Kept at 50 to match the legacy production constant — the
// detector_test.go test constant mirrors this.
const cheatscoreFlagThreshold = 50.0

// publishOptions carries every value cheatscorePublish needs from the
// pipeline in one struct.
type publishOptions struct {
	channels []Channel
	combined float64 // pre-boost composite, [0, 100]

	wingmanBoosted   bool
	wingmanReason    string
	competitiveBoost bool
	positionDiscount float64

	evidenceStacking      bool
	evidenceStackingCount int
	ttdSub100Floor        bool

	sniperOverrides []string

	finalLikelihood float64 // [0, 100] after all overrides + boosts
}

// channelLegacyKey maps a channel ID to the legacy anti_cheat key under which
// its Score is published. Channels not in this map don't get a *_score key —
// they publish under <id>_score generically. This preserves backward
// compatibility for hs_score, snap_score, reaction_score, recoil_score.
var channelLegacyKey = map[string]string{
	"hs":       "hs_score",
	"snap":     "snap_score",
	"reaction": "reaction_score", // ttd_p10
	"recoil":   "recoil_score",
}

// cheatscorePublish writes all anti_cheat metrics for one player. Each
// channel emits three keys (<id>_score, <id>_confidence, <id>_zone) plus the
// legacy alias if one exists.
func cheatscorePublish(ps *PlayerStats, opt publishOptions) {
	ps.AddMetric(cheatscoreCategoryAntiCheat, Key("cheat_likelihood"), Metric{
		Type:        MetricPercentage,
		FloatValue:  opt.finalLikelihood,
		Description: "Estimated likelihood of player cheating",
	})

	// Per-channel transparency: <id>_score, <id>_confidence, <id>_zone.
	// Channels with HasData=false still emit zero values for the score key so
	// the table layout stays consistent.
	for _, ch := range opt.channels {
		baseID := ch.ID
		score := ch.Score
		conf := ch.Confidence
		zone := ch.Zone

		// Legacy alias (hs/snap/reaction/recoil keep their old names too).
		if legacyKey, ok := channelLegacyKey[baseID]; ok {
			ps.AddMetric(cheatscoreCategoryAntiCheat, Key(legacyKey), Metric{
				Type:        MetricFloat,
				FloatValue:  score,
				Description: fmt.Sprintf("%s cheat score component (0-1)", baseID),
			})
		}

		// Generic <id>_score for non-legacy channels and as the canonical key
		// for the new ones.
		if _, hasLegacy := channelLegacyKey[baseID]; !hasLegacy {
			ps.AddMetric(cheatscoreCategoryAntiCheat, Key(baseID+"_score"), Metric{
				Type:        MetricFloat,
				FloatValue:  score,
				Description: fmt.Sprintf("%s cheat score component (0-1)", baseID),
			})
		}

		ps.AddMetric(cheatscoreCategoryAntiCheat, Key(baseID+"_confidence"), Metric{
			Type:        MetricFloat,
			FloatValue:  conf,
			Description: fmt.Sprintf("Confidence in %s reading (0-1)", baseID),
		})
		ps.AddMetric(cheatscoreCategoryAntiCheat, Key(baseID+"_zone"), Metric{
			Type:        MetricString,
			StringValue: zone.String(),
			Description: fmt.Sprintf("Interpretation band for %s", baseID),
		})
	}

	ps.AddMetric(cheatscoreCategoryAntiCheat, Key("total_cheat_score"), Metric{
		Type:        MetricFloat,
		FloatValue:  opt.combined / 100.0,
		Description: "Pre-boost combined Bayesian likelihood (0-1)",
	})

	if opt.wingmanBoosted {
		ps.AddMetric(cheatscoreCategoryAntiCheat, Key("wingman_boost"), Metric{
			Type:        MetricString,
			StringValue: "Yes",
			Description: "Wingman boost applied (" + opt.wingmanReason + ")",
		})
		ps.AddMetric(cheatscoreCategoryAntiCheat, Key("wingman_kpr_boost_reason"), Metric{
			Type:        MetricString,
			StringValue: opt.wingmanReason,
			Description: "Reason the Wingman boost fired",
		})
	}

	if opt.competitiveBoost {
		ps.AddMetric(cheatscoreCategoryAntiCheat, Key("competitive_boost"), Metric{
			Type:        MetricString,
			StringValue: "Yes",
			Description: "Player has more than 39 kills in regulation time (20% boost applied)",
		})
	}

	if opt.positionDiscount > 0 {
		ps.AddMetric(cheatscoreCategoryAntiCheat, Key("position_discount"), Metric{
			Type:        MetricPercentage,
			FloatValue:  opt.positionDiscount * 100,
			Description: "Reduction applied for low scoreboard position vs. teammates",
		})
	}

	if opt.evidenceStacking {
		ps.AddMetric(cheatscoreCategoryAntiCheat, Key("evidence_stacking_boost"), Metric{
			Type:        MetricString,
			StringValue: fmt.Sprintf("Yes (%d strong channels)", opt.evidenceStackingCount),
			Description: "×1.4 boost — ≥3 channels with score×confidence ≥0.30",
		})
	}

	if opt.ttdSub100Floor {
		ps.AddMetric(cheatscoreCategoryAntiCheat, Key("ttd_sub100_high_floor"), Metric{
			Type:        MetricString,
			StringValue: "Yes",
			Description: "Score floored at 55% — sub-100ms TTD rate ≥25% with ≥3 samples",
		})
	}

	for _, name := range opt.sniperOverrides {
		ps.AddMetric(cheatscoreCategoryAntiCheat, Key(name), Metric{
			Type:        MetricString,
			StringValue: "Yes",
			Description: "Sniper-anomaly override — pinned to 100%",
		})
	}

	flag := "No"
	if opt.finalLikelihood >= cheatscoreFlagThreshold {
		flag = "Yes"
	}
	ps.AddMetric(cheatscoreCategoryAntiCheat, Key("cheater"), Metric{
		Type:        MetricString,
		StringValue: flag,
		Description: "Flag — Yes if cheat_likelihood ≥ flagThreshold",
	})
}
