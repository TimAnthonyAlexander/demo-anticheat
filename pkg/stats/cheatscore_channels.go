package stats

// cheatscore_channels.go: one evaluate*() function per cheat-score channel.
// PR2 wires 10 channels total:
//
//   - hs                 — headshot % (bidirectional)
//   - snap               — P95 snap velocity (positive-only)
//   - reaction (ttd_p10) — P10 time-to-damage (bidirectional)
//   - ttd_sub100         — sub-100 ms TTD rate (positive-only, count-pinned conf)
//   - recoil             — recoil_score passthrough (positive-only)
//   - pre_fov            — pre-FOV pre-aim median angle (bidirectional)
//   - pre_fov_presence   — sample count + lobby asymmetry (positive-only;
//     evaluated in cheatscore_combiner.go because it needs lobby context)
//   - attention          — nearest-enemy angle median (positive-only)
//   - back_killed        — back-killed % (positive-only)
//   - decoupling         — attention − pre_fov delta (positive-only)
//
// Each evaluator returns a Channel; channels missing required inputs return
// HasData=false and contribute nothing to the combiner.

const (
	channelCategoryKills      = Category("kills")
	channelCategoryAiming     = Category("aiming")
	channelCategoryReaction   = Category("reaction")
	channelCategoryRecoil     = Category("recoil")
	channelCategoryBehavioral = Category("behavioral")
)

// evaluateHS scores headshot percentage. Ramp 55%→75%, n_full=20.
// Bidirectional: a low HS% on many kills is real evidence of cleanness.
func evaluateHS(ps *PlayerStats) Channel {
	totalKills, hasKills := psGetInt(ps, channelCategoryKills, Key("total_kills"))
	if !hasKills || totalKills <= 0 {
		return Channel{ID: "hs", Weight: 0.18, Mode: bidirectional}
	}
	hsPct, _ := psGetFloat(ps, channelCategoryKills, Key("headshot_percentage"))
	score := linearScore(hsPct, 55.0, 75.0)
	return Channel{
		ID:         "hs",
		Score:      score,
		Confidence: linearConfidence(totalKills, 20),
		Raw:        hsPct,
		SampleN:    totalKills,
		Weight:     0.18,
		Zone:       zoneFor(score),
		Mode:       bidirectional,
		HasData:    true,
	}
}

// evaluateSnap scores P95 snap velocity. Ramp 2.0→3.5 °/ms, n_full=10.
// Positive-only: a low P95 doesn't exonerate, only flags upward.
func evaluateSnap(ps *PlayerStats) Channel {
	snapCount, hasN := psGetInt(ps, channelCategoryAiming, Key("snap_count"))
	if !hasN || snapCount <= 0 {
		return Channel{ID: "snap", Weight: 0.12, Mode: positiveOnly}
	}
	p95, _ := psGetFloat(ps, channelCategoryAiming, Key("p95_snap_velocity"))
	score := linearScore(p95, 2.0, 3.5)
	return Channel{
		ID:         "snap",
		Score:      score,
		Confidence: linearConfidence(snapCount, 10),
		Raw:        p95,
		SampleN:    snapCount,
		Weight:     0.12,
		Zone:       zoneFor(score),
		Mode:       positiveOnly,
		HasData:    true,
	}
}

// evaluateTTDP10 scores P10 time-to-damage. Ramp 400→100 ms, n_full=10,
// sqrt confidence. Bidirectional: a 600ms P10 on many samples is real
// evidence of human-level reactions.
func evaluateTTDP10(ps *PlayerStats) Channel {
	n, hasN := psGetInt(ps, channelCategoryReaction, Key("ttd_samples"))
	if !hasN || n <= 0 {
		return Channel{ID: "reaction", Weight: 0.10, Mode: bidirectional}
	}
	p10, _ := psGetFloat(ps, channelCategoryReaction, Key("p10_ttd"))
	score := linearScore(p10, 400.0, 100.0) // descending ramp: low ms → high score
	return Channel{
		ID:         "reaction",
		Score:      score,
		Confidence: sqrtConfidence(n, 10),
		Raw:        p10,
		SampleN:    n,
		Weight:     0.10,
		Zone:       zoneFor(score),
		Mode:       bidirectional,
		HasData:    true,
	}
}

// evaluateTTDSub100 scores the sub-100ms TTD rate. Ramp 2%→30%, n_full=30,
// sqrt confidence — pinned to 1.0 when count_sub100 ≥ 2 (two or more sub-
// 100ms damage events in a single match is the surprising signal, not the
// rate). Positive-only.
func evaluateTTDSub100(ps *PlayerStats) Channel {
	n, hasN := psGetInt(ps, channelCategoryReaction, Key("ttd_samples"))
	if !hasN || n <= 0 {
		return Channel{ID: "ttd_sub100", Weight: 0.10, Mode: positiveOnly}
	}
	rate, _ := psGetFloat(ps, channelCategoryReaction, Key("sub_100ms_ttd"))
	score := linearScore(rate, 2.0, 30.0)
	conf := sqrtConfidence(n, 30)
	countSub100 := int64(rate / 100.0 * float64(n))
	if countSub100 >= 2 {
		conf = 1.0
	}
	return Channel{
		ID:         "ttd_sub100",
		Score:      score,
		Confidence: conf,
		Raw:        rate,
		SampleN:    n,
		Weight:     0.10,
		Zone:       zoneFor(score),
		Mode:       positiveOnly,
		HasData:    true,
	}
}

// evaluateRecoil reuses the already-computed recoil_score (0–1, where 1 is
// suspicious). Confidence ramps over 20 counted bullets. Positive-only — a
// clean recoil reading just means we didn't see suspicious sprays.
func evaluateRecoil(ps *PlayerStats) Channel {
	raw, ok := psGetFloat(ps, channelCategoryRecoil, Key("recoil_score"))
	bullets, _ := psGetInt(ps, channelCategoryRecoil, Key("total_counted_bullets"))
	if !ok || bullets <= 0 {
		return Channel{ID: "recoil", Weight: 0.10, Mode: positiveOnly}
	}
	score := clamp01(raw)
	return Channel{
		ID:         "recoil",
		Score:      score,
		Confidence: linearConfidence(bullets, 20),
		Raw:        raw,
		SampleN:    bullets,
		Weight:     0.10,
		Zone:       zoneFor(score),
		Mode:       positiveOnly,
		HasData:    true,
	}
}

// evaluatePreFOV scores pre-FOV pre-aim median angle. Ramp 12°→4° (clean→
// blatant — descending). n_full=15, sqrt confidence. Bidirectional.
//
// Calibration deviates from the user-supplied research table (7°→2.5°) because
// observed wingman cheaters land at 6.17° and 7.25° — the 7° clean threshold
// would zero them out. The 12°→4° ramp is anchored on the observed corpus.
func evaluatePreFOV(ps *PlayerStats) Channel {
	n, hasN := psGetInt(ps, channelCategoryBehavioral, Key("pre_fov_aim_samples"))
	if !hasN || n <= 0 {
		return Channel{ID: "pre_fov", Weight: 0.20, Mode: bidirectional}
	}
	med, _ := psGetFloat(ps, channelCategoryBehavioral, Key("pre_fov_aim_median_deg"))
	score := linearScore(med, 12.0, 4.0)
	return Channel{
		ID:         "pre_fov",
		Score:      score,
		Confidence: sqrtConfidence(n, 15),
		Raw:        med,
		SampleN:    n,
		Weight:     0.20,
		Zone:       zoneFor(score),
		Mode:       bidirectional,
		HasData:    true,
	}
}

// evaluateAttention scores nearest-enemy angle median during off-engagement
// moments. Ramp 33°→18° (clean→blatant — descending). n_full=200 frames.
// Positive-only — a high attention angle just means crosshair isn't tight,
// which isn't exoneration.
func evaluateAttention(ps *PlayerStats) Channel {
	n, hasN := psGetInt(ps, channelCategoryBehavioral, Key("nearest_enemy_angle_samples"))
	if !hasN || n <= 0 {
		return Channel{ID: "attention", Weight: 0.06, Mode: positiveOnly}
	}
	med, _ := psGetFloat(ps, channelCategoryBehavioral, Key("nearest_enemy_angle_median_deg"))
	score := linearScore(med, 33.0, 18.0)
	return Channel{
		ID:         "attention",
		Score:      score,
		Confidence: linearConfidence(n, 200),
		Raw:        med,
		SampleN:    n,
		Weight:     0.06,
		Zone:       zoneFor(score),
		Mode:       positiveOnly,
		HasData:    true,
	}
}

// evaluateBackKilled scores back-killed avoidance. Ramp 25%→3% (clean→blatant
// — descending; low back-killed rate is suspicious). n_full=8 deaths.
func evaluateBackKilled(ps *PlayerStats) Channel {
	n, hasN := psGetInt(ps, channelCategoryBehavioral, Key("back_killed_total_deaths"))
	if !hasN || n <= 0 {
		return Channel{ID: "back_killed", Weight: 0.06, Mode: positiveOnly}
	}
	rate, _ := psGetFloat(ps, channelCategoryBehavioral, Key("back_killed_pct"))
	score := linearScore(rate, 25.0, 3.0)
	return Channel{
		ID:         "back_killed",
		Score:      score,
		Confidence: linearConfidence(n, 8),
		Raw:        rate,
		SampleN:    n,
		Weight:     0.06,
		Zone:       zoneFor(score),
		Mode:       positiveOnly,
		HasData:    true,
	}
}

// evaluateDecoupling scores the fight-vs-idle decoupling: attention_median −
// pre_fov_median. Wallhackers concentrate during engagements but their
// crosshair drifts during chill moments; legit players are consistent.
// Ramp 8°→22°, positive-only. Silent if either half is missing.
func evaluateDecoupling(ps *PlayerStats) Channel {
	preFOVN, hasFOVN := psGetInt(ps, channelCategoryBehavioral, Key("pre_fov_aim_samples"))
	attN, hasAttN := psGetInt(ps, channelCategoryBehavioral, Key("nearest_enemy_angle_samples"))
	if !hasFOVN || preFOVN <= 0 || !hasAttN || attN <= 0 {
		return Channel{ID: "decoupling", Weight: 0.10, Mode: positiveOnly}
	}
	preFOVMed, _ := psGetFloat(ps, channelCategoryBehavioral, Key("pre_fov_aim_median_deg"))
	attMed, _ := psGetFloat(ps, channelCategoryBehavioral, Key("nearest_enemy_angle_median_deg"))
	delta := attMed - preFOVMed
	score := linearScore(delta, 8.0, 22.0)

	// Confidence requires both halves trustworthy.
	confFOV := sqrtConfidence(preFOVN, 15)
	confAtt := linearConfidence(attN, 200)
	conf := confFOV
	if confAtt < conf {
		conf = confAtt
	}
	return Channel{
		ID:         "decoupling",
		Score:      score,
		Confidence: conf,
		Raw:        delta,
		SampleN:    preFOVN, // pick one — the limiting half is what gates confidence anyway
		Weight:     0.10,
		Zone:       zoneFor(score),
		Mode:       positiveOnly,
		HasData:    true,
	}
}

// evaluateChannelsForPlayer runs the 9 lobby-independent channels for one
// player. pre_fov_presence is added in the combiner after the lobby context
// is available.
func evaluateChannelsForPlayer(ps *PlayerStats) []Channel {
	return []Channel{
		evaluateHS(ps),
		evaluateSnap(ps),
		evaluateTTDP10(ps),
		evaluateTTDSub100(ps),
		evaluateRecoil(ps),
		evaluatePreFOV(ps),
		evaluateAttention(ps),
		evaluateBackKilled(ps),
		evaluateDecoupling(ps),
	}
}
