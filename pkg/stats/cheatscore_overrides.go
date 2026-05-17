package stats

import "fmt"

// PR2 overrides:
//   - applyWingmanBoost: KPR-based rule (or ≥10 kills backstop) replaces the
//     legacy strict >10-kills gate. Short Wingman demos that end at 8–9 rounds
//     now still receive the boost when KPR is suspicious.
//   - applyEvidenceStacking: ×1.4 boost when ≥3 channels each have
//     score × confidence ≥ 0.30 — three independent moderate signals are
//     stronger evidence than the raw arithmetic sum suggests.
//   - applyTTDSub100Floor: floor at 55% when sub_100ms_ttd ≥ 25% AND
//     ttd_samples ≥ 3. Sub-100ms TTD at any meaningful rate is statistically
//     implausible without info or aim assistance (Leetify Public Data Library).
//     Applied AFTER boosts so it caps regression, not amplification.

const (
	cheatscoreCategoryGameInfo  = Category("game_info")
	cheatscoreCategoryAntiCheat = Category("anti_cheat")

	wingmanKPRThreshold        = 0.7
	evidenceStackingMinStrong  = 3
	evidenceStackingMinChannel = 0.30
	evidenceStackingMultiplier = 1.4
	ttdSub100FloorRate         = 25.0
	ttdSub100FloorSamples      = 3
	ttdSub100FloorScore        = 55.0
)

// applyWingmanBoost: ×1.8 in Wingman when KPR ≥ 0.7 OR kills ≥ 10.
// Returns (new score, fired, human-readable reason).
func applyWingmanBoost(score float64, ps *PlayerStats) (float64, bool, string) {
	gameMode, _ := psGetString(ps, cheatscoreCategoryGameInfo, Key("game_mode"))
	if gameMode != "Wingman" {
		return score, false, ""
	}
	totalKills, _ := psGetInt(ps, channelCategoryKills, Key("total_kills"))
	roundCount, _ := psGetInt(ps, cheatscoreCategoryGameInfo, Key("round_count"))

	var kpr float64
	if roundCount > 0 {
		kpr = float64(totalKills) / float64(roundCount)
	}

	if totalKills >= 10 {
		return score * 1.8, true, fmt.Sprintf("kills=%d", totalKills)
	}
	if kpr >= wingmanKPRThreshold {
		return score * 1.8, true, fmt.Sprintf("KPR=%.2f", kpr)
	}
	return score, false, ""
}

// applyCompetitiveBoost: ×1.2 in Competitive when totalKills > 39 in ≤30 rounds.
func applyCompetitiveBoost(score float64, ps *PlayerStats) (float64, bool) {
	gameMode, _ := psGetString(ps, cheatscoreCategoryGameInfo, Key("game_mode"))
	if gameMode != "Competitive" {
		return score, false
	}
	totalKills, _ := psGetInt(ps, channelCategoryKills, Key("total_kills"))
	roundCount, _ := psGetInt(ps, cheatscoreCategoryGameInfo, Key("round_count"))
	if totalKills <= 39 || roundCount > 30 {
		return score, false
	}
	return score * 1.2, true
}

// applyPositionDiscount multiplies score by (1 - 0.2 × position_factor).
func applyPositionDiscount(score float64, ps *PlayerStats) (float64, float64) {
	factor, ok := psGetFloat(ps, scoreboardCategory, Key("position_factor"))
	if !ok || factor <= 0 {
		return score, 0
	}
	discount := 0.20 * factor
	return score * (1.0 - discount), discount
}

// applyEvidenceStacking: ×1.4 when ≥3 channels each have score × conf ≥ 0.30.
// Encodes the Bayesian intuition that independent moderate signals compound.
func applyEvidenceStacking(score float64, channels []Channel) (float64, bool, int) {
	strong := 0
	for _, ch := range channels {
		if !ch.HasData {
			continue
		}
		if ch.Score*ch.Confidence >= evidenceStackingMinChannel {
			strong++
		}
	}
	if strong >= evidenceStackingMinStrong {
		return score * evidenceStackingMultiplier, true, strong
	}
	return score, false, strong
}

// applyTTDSub100Floor: floor at 55% when a player exhibits ALL of:
//   - sub_100ms_ttd ≥ 25% on ≥3 TTD samples (triggerbot-style reactions)
//   - pre-FOV median ≤ 10° on ≥3 pre-FOV samples (wallhack-style pre-aim)
//   - lobby asymmetry: at least half of OTHER players have <2 pre-FOV samples
//
// The asymmetry requirement avoids false-positives in 5v5 pro lobbies where
// every player has pre-FOV samples by virtue of long matches. A wallhack
// player in a 2v2 stands out because their teammates and opponents don't
// produce the same pre-aim signal; a pro doesn't.
//
// Applied AFTER boosts so it caps regression, not amplification.
func applyTTDSub100Floor(score float64, ps *PlayerStats, preFOVLobbyAsymmetric bool) (float64, bool) {
	n, hasN := psGetInt(ps, channelCategoryReaction, Key("ttd_samples"))
	if !hasN || n < ttdSub100FloorSamples {
		return score, false
	}
	rate, hasRate := psGetFloat(ps, channelCategoryReaction, Key("sub_100ms_ttd"))
	if !hasRate || rate < ttdSub100FloorRate {
		return score, false
	}

	preFOVN, hasFOVN := psGetInt(ps, channelCategoryBehavioral, Key("pre_fov_aim_samples"))
	if !hasFOVN || preFOVN < 3 {
		return score, false
	}
	preFOVMed, hasFOVMed := psGetFloat(ps, channelCategoryBehavioral, Key("pre_fov_aim_median_deg"))
	if !hasFOVMed || preFOVMed <= 0 || preFOVMed > 10.0 {
		return score, false
	}

	if !preFOVLobbyAsymmetric {
		return score, false
	}

	if score < ttdSub100FloorScore {
		return ttdSub100FloorScore, true
	}
	return score, true
}

// applySniperOverrides pins the score to 100 for Tim's custom high-confidence
// sniper anomalies. Returns (new score, list of triggered override names).
func applySniperOverrides(score float64, ps *PlayerStats) (float64, []string) {
	triggered := []string{}

	if wb, ok := psGetInt(ps, sniperCategory, Key("sniper_wallbang_kills")); ok && wb > 10 {
		score = 100.0
		triggered = append(triggered, "sniper_wallbang_override")
	}

	if scoutKills, ok := psGetInt(ps, sniperCategory, Key("scout_kills")); ok && scoutKills > 10 {
		if scoutHS, ok := psGetFloat(ps, sniperCategory, Key("scout_hs_rate")); ok && scoutHS >= 80.0 {
			score = 100.0
			triggered = append(triggered, "scout_precision_override")
		}
	}

	return score, triggered
}
