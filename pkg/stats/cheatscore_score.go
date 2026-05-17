package stats

// cheatscoreEvaluate orchestrates the scoring pipeline across every player.
//
// PR2 pipeline:
//  1. Evaluate the 9 lobby-independent channels for every player.
//  2. Append pre_fov_presence (lobby-dependent) for every player.
//  3. Lobby-relative normalize each channel.
//  4. Per player:
//     a. Combine via Bayesian log-odds → pre-boost likelihood [0, 100].
//     b. Wingman KPR boost (×1.8) or Competitive boost (×1.2).
//     c. Scoreboard-position discount (×(1 − 0.2·factor)).
//     d. Evidence-stacking boost (×1.4 when ≥3 channels strong).
//     e. TTD-sub100 high floor (max(score, 55) when rate ≥25% on ≥3 samples).
//     f. Sniper overrides (pin to 100 when triggered).
//     g. Clamp to [0, 100].
//     h. Publish all metrics.
func cheatscoreEvaluate(demoStats *DemoStats) {
	if demoStats == nil || len(demoStats.Players) == 0 {
		return
	}

	// Pass 1: per-player channel evaluation.
	perPlayer := make(map[uint64][]Channel, len(demoStats.Players))
	for sid, ps := range demoStats.Players {
		perPlayer[sid] = evaluateChannelsForPlayer(ps)
	}

	// Pre-compute lobby pre-FOV tally — used by both pre_fov_presence and
	// the TTD-sub100 co-occurrence floor.
	samplesBySID, asymBySID := preFOVLobbyTally(demoStats)

	// Pass 2: lobby-dependent pre_fov_presence channel.
	cheatscoreAddPreFOVPresence(demoStats, perPlayer, samplesBySID, asymBySID)

	// Pass 3: lobby-relative trimmed-mean shrinkage across all channels.
	cheatscoreNormalizeLobby(perPlayer)

	// Pass 4: combine + boosts + publish.
	for sid, ps := range demoStats.Players {
		channels := perPlayer[sid]
		if channels == nil {
			channels = []Channel{}
		}

		combined := cheatscoreBayesianCombine(channels)

		score, wingmanApplied, wingmanReason := applyWingmanBoost(combined, ps)
		score, competitiveApplied := applyCompetitiveBoost(score, ps)
		score, discount := applyPositionDiscount(score, ps)
		score, stackApplied, stackCount := applyEvidenceStacking(score, channels)
		score, floorApplied := applyTTDSub100Floor(score, ps, asymBySID[sid])
		if score > 100.0 {
			score = 100.0
		}
		score, sniperOverrides := applySniperOverrides(score, ps)

		cheatscorePublish(ps, publishOptions{
			channels:              channels,
			combined:              combined,
			wingmanBoosted:        wingmanApplied,
			wingmanReason:         wingmanReason,
			competitiveBoost:      competitiveApplied,
			positionDiscount:      discount,
			evidenceStacking:      stackApplied,
			evidenceStackingCount: stackCount,
			ttdSub100Floor:        floorApplied,
			sniperOverrides:       sniperOverrides,
			finalLikelihood:       score,
		})
	}
}
