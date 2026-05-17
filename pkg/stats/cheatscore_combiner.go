package stats

import (
	"math"
	"sort"
)

// PR2 combiner: Bayesian log-odds combination, lobby-relative normalization,
// and the lobby-dependent pre_fov_presence channel.
//
// The combiner takes a slice of evaluated channels for one player and returns
// a percentage [0, 100]. Channels with !HasData contribute nothing. Channel
// Mode controls whether negative log-odds count:
//   - bidirectional: a clean reading produces genuine negative evidence.
//   - positiveOnly:  a clean reading contributes 0; only suspicious values
//     shift the score upward.

const (
	// cheatscorePrior is the base-rate cheater probability before any evidence
	// is seen. 10% chosen so a complete absence of signal pulls combined
	// likelihood toward ~10%, not 50%.
	cheatscorePrior = 0.10

	// logitEpsilon clamps scores away from 0/1 before logit() so contributions
	// are bounded and don't dominate the sum.
	logitEpsilon = 0.02

	// lobbyNormAlpha is the shrinkage factor applied to the lobby trimmed
	// mean when adjusting individual scores.
	lobbyNormAlpha = 0.4

	// lobbyNormMinConf is the minimum confidence required for a player's
	// channel reading to count toward the lobby baseline.
	lobbyNormMinConf = 0.25
)

func cheatscoreLogit(p float64) float64 {
	if p < logitEpsilon {
		p = logitEpsilon
	}
	if p > 1.0-logitEpsilon {
		p = 1.0 - logitEpsilon
	}
	return math.Log(p / (1.0 - p))
}

func cheatscoreSigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// cheatscoreBayesianCombine returns the combined cheat likelihood [0, 100]
// for one player from a slice of channels.
func cheatscoreBayesianCombine(channels []Channel) float64 {
	logOdds := cheatscoreLogit(cheatscorePrior)
	for _, ch := range channels {
		if !ch.HasData || ch.Confidence <= 0 || ch.Weight <= 0 {
			continue
		}
		contrib := ch.Weight * ch.Confidence * cheatscoreLogit(ch.Score)
		if ch.Mode == positiveOnly && contrib < 0 {
			contrib = 0
		}
		logOdds += contrib
	}
	return cheatscoreSigmoid(logOdds) * 100.0
}

// cheatscoreNormalizeLobby applies the lobby-relative trimmed-mean shrinkage
// across all players in perPlayer for every channel that has ≥2 contributors.
//
// For each channel ID:
//  1. Collect scores from players with HasData AND Confidence ≥ lobbyNormMinConf.
//  2. If at least 2 contributors: drop the highest, take mean → μ_trim.
//  3. Adjust every contributor's score: clamp01(score − α·μ_trim).
//
// The trim prevents a lone cheater dragging the baseline up and depressing
// their own outlier signal. When everyone in the lobby reads clean on a
// channel, μ_trim is small and the shrinkage is negligible — addressing the
// "clean lobby should pull scores down" requirement only on channels where
// the lobby actually has elevated readings.
func cheatscoreNormalizeLobby(perPlayer map[uint64][]Channel) {
	if len(perPlayer) == 0 {
		return
	}
	// Channel IDs we'll iterate (stable order derived from any player's slice).
	var ids []string
	for _, channels := range perPlayer {
		for _, ch := range channels {
			ids = append(ids, ch.ID)
		}
		break
	}

	for _, id := range ids {
		// Collect scores meeting confidence floor.
		var contributors []float64
		for _, channels := range perPlayer {
			for _, ch := range channels {
				if ch.ID != id || !ch.HasData || ch.Confidence < lobbyNormMinConf {
					continue
				}
				contributors = append(contributors, ch.Score)
			}
		}
		if len(contributors) < 2 {
			continue
		}
		// Trim: drop the single highest score, take mean of the rest.
		sort.Float64s(contributors)
		trimmed := contributors[:len(contributors)-1]
		sum := 0.0
		for _, v := range trimmed {
			sum += v
		}
		muTrim := sum / float64(len(trimmed))

		// Apply shrinkage in-place.
		for sid, channels := range perPlayer {
			for i, ch := range channels {
				if ch.ID != id || !ch.HasData {
					continue
				}
				adjusted := clamp01(ch.Score - lobbyNormAlpha*muTrim)
				channels[i].Score = adjusted
				channels[i].Zone = zoneFor(adjusted)
			}
			perPlayer[sid] = channels
		}
	}
}

// preFOVLobbyTally returns the per-player pre-FOV sample counts and a
// per-player flag indicating whether the rest of the lobby is asymmetric in
// pre-FOV samples (≥50% of OTHER players have <2 samples). Used by both the
// pre_fov_presence channel and the TTD-sub100 floor.
func preFOVLobbyTally(demoStats *DemoStats) (samplesBySID map[uint64]int64, asymBySID map[uint64]bool) {
	samplesBySID = map[uint64]int64{}
	for sid, ps := range demoStats.Players {
		if sid == 0 {
			continue
		}
		n, _ := psGetInt(ps, channelCategoryBehavioral, Key("pre_fov_aim_samples"))
		samplesBySID[sid] = n
	}

	asymBySID = map[uint64]bool{}
	for sid := range samplesBySID {
		others, lowSampleOthers := 0, 0
		for otherSID, otherN := range samplesBySID {
			if otherSID == sid {
				continue
			}
			others++
			if otherN < 2 {
				lowSampleOthers++
			}
		}
		asymBySID[sid] = others >= 2 && float64(lowSampleOthers)/float64(others) >= 0.5
	}
	return samplesBySID, asymBySID
}

// cheatscoreAddPreFOVPresence appends a pre_fov_presence channel to each
// player's slice. Fires only when:
//   - player has ≥4 pre_fov samples
//   - player's pre_fov median ≤ 10° (tight pre-aim, not random)
//   - lobby asymmetry: ≥50% of OTHER lobby members have <2 pre_fov samples
//
// Score ramps 3 samples → 0.5, 6 samples → 1.0. Confidence pinned to 1.0
// when the rule fires; otherwise the channel is added with HasData=false so
// publish.go still emits the per-channel transparency keys.
func cheatscoreAddPreFOVPresence(demoStats *DemoStats, perPlayer map[uint64][]Channel, samplesBySID map[uint64]int64, asymBySID map[uint64]bool) {
	for sid, ps := range demoStats.Players {
		if sid == 0 {
			perPlayer[sid] = append(perPlayer[sid], Channel{ID: "pre_fov_presence", Weight: 0.10, Mode: positiveOnly})
			continue
		}

		n := samplesBySID[sid]
		med, _ := psGetFloat(ps, channelCategoryBehavioral, Key("pre_fov_aim_median_deg"))

		fires := n >= 4 && med > 0 && med <= 10.0 && asymBySID[sid]

		ch := Channel{
			ID:     "pre_fov_presence",
			Weight: 0.10,
			Mode:   positiveOnly,
		}
		if fires {
			// Score: 0.5 at n=3, 1.0 at n=6, linear; clamped.
			score := clamp01(0.5 + float64(n-3)/3.0*0.5)
			ch.HasData = true
			ch.Score = score
			ch.Confidence = 1.0
			ch.Raw = float64(n)
			ch.SampleN = n
			ch.Zone = zoneFor(score)
		}
		perPlayer[sid] = append(perPlayer[sid], ch)
	}
}
