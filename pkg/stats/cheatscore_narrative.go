package stats

import (
	"fmt"
	"sort"
	"strings"
)

// cheatscore_narrative.go produces a short prose paragraph summarizing each
// player's cheat-detection profile. One sentence per channel whose RAW value
// crosses an objectively-anchored threshold, ordered strongest first. A
// closing sentence notes any boosts that fired.
//
// Anchoring adjectives on raw values (not on the lobby-normalized score)
// keeps clean players' narratives honest: in a pro 5v5 lobby every channel
// scores high after normalization, but raw values still tell the truth. A
// pre-FOV median of 9° is normal regardless of what the post-norm score says.
//
// Three tiers per channel:
//
//	exceptionalThreshold → "exceptionally" / "blatantly" / "unmistakably"
//	strongThreshold      → "strongly" / "markedly" / "clearly" / "notably"
//	mildThreshold        → "moderately" / "appreciably" / "slightly"
//	below                → sentence omitted entirely
//
// Per-player, adjectives cycle within each tier so the paragraph doesn't read
// "strongly X, strongly Y, strongly Z."

var (
	narrativeBlatantAdjs = []string{"exceptionally", "blatantly", "unmistakably"}
	narrativeStrongAdjs  = []string{"strongly", "markedly", "clearly", "notably"}
	narrativeMildAdjs    = []string{"moderately", "appreciably", "slightly"}
)

// narrativeTier returns the tier index (3=blatant, 2=strong, 1=mild, 0=skip)
// for a raw value compared to three thresholds. `ascendingSuspicion` is true
// for metrics where higher raw values are more suspicious (HS%, snap velocity,
// sub-100ms TTD rate); false for metrics where lower is more suspicious
// (pre-FOV median, P10 TTD, back-killed rate).
func narrativeTier(raw, mild, strong, blatant float64, ascendingSuspicion bool) int {
	if ascendingSuspicion {
		switch {
		case raw >= blatant:
			return 3
		case raw >= strong:
			return 2
		case raw >= mild:
			return 1
		default:
			return 0
		}
	}
	switch {
	case raw <= blatant:
		return 3
	case raw <= strong:
		return 2
	case raw <= mild:
		return 1
	default:
		return 0
	}
}

type narrativeChannel struct {
	id       string
	tier     int     // 0=skip, 1=mild, 2=strong, 3=blatant
	raw      float64
	sampleN  int64
}

// buildCheatscoreNarrative reads a player's published anti_cheat metrics and
// returns a multi-sentence paragraph. Returns "" if the player has no
// anti_cheat data (e.g., parser failed).
func buildCheatscoreNarrative(ps *PlayerStats) string {
	if ps == nil {
		return ""
	}

	channels := collectNarrativeChannels(ps)

	// Keep only channels whose raw value crossed at least the mild threshold,
	// then sort strongest tier first.
	filtered := make([]narrativeChannel, 0, len(channels))
	for _, c := range channels {
		if c.tier > 0 {
			filtered = append(filtered, c)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].tier > filtered[j].tier })

	sentences := make([]string, 0, len(filtered)+2)
	blatantIdx, strongIdx, mildIdx := 0, 0, 0
	for _, c := range filtered {
		var adj string
		switch c.tier {
		case 3:
			adj = narrativeBlatantAdjs[blatantIdx%len(narrativeBlatantAdjs)]
			blatantIdx++
		case 2:
			adj = narrativeStrongAdjs[strongIdx%len(narrativeStrongAdjs)]
			strongIdx++
		default:
			adj = narrativeMildAdjs[mildIdx%len(narrativeMildAdjs)]
			mildIdx++
		}
		if s := narrativeSentence(c, adj); s != "" {
			sentences = append(sentences, s)
		}
	}

	// Closing sentences for boosts that fired. Co-occurrence subsumes evidence
	// stacking narratively, so don't repeat both.
	coOccur := psHasYes(ps, Key("wallhack_co_occurrence_boost"))
	switch {
	case psHasYes(ps, Key("sniper_wallbang_override")) || psHasYes(ps, Key("scout_precision_override")):
		sentences = append(sentences, "A sniper-anomaly override pinned likelihood to 100%.")
	case coOccur:
		sentences = append(sentences, "The wallhack co-occurrence pattern triggered — both pre-FOV pre-aim AND elevated back-kill-given rate together, the wallhack-via-info signature.")
	case psHasYes(ps, Key("evidence_stacking_boost")):
		sentences = append(sentences, "Multiple strong channels co-occur, triggering the evidence-stacking boost.")
	}
	if psHasYes(ps, Key("ttd_sub100_high_floor")) {
		sentences = append(sentences, "The sub-100 ms time-to-damage floor enforced a 55% minimum likelihood.")
	}
	if psHasYes(ps, Key("wingman_boost")) {
		sentences = append(sentences, "A Wingman-match KPR boost was applied to reflect the short-format pace.")
	}

	if len(sentences) == 0 {
		return "No suspicious signals registered across the evaluated channels."
	}
	return strings.Join(sentences, " ")
}

// channelRaw fetches the raw value and sample count for a given channel from
// PlayerStats. Returns (raw, n, ok) — ok=false if the metric doesn't exist
// (e.g., n=0 case for back_killed when there's not enough data).
func channelRaw(ps *PlayerStats, rawCat Category, rawK Key, nCat Category, nK Key) (float64, int64, bool) {
	m, ok := ps.GetMetric(rawCat, rawK)
	if !ok {
		return 0, 0, false
	}
	n, _ := psGetInt(ps, nCat, nK)
	return m.FloatValue, n, true
}

func collectNarrativeChannels(ps *PlayerStats) []narrativeChannel {
	out := make([]narrativeChannel, 0, 10)

	// HS%: ascending suspicion. Tier breakpoints aim at "above-average rifler",
	// "highly accurate", "headshot-machine" rather than chasing the channel's
	// 55→75 ramp directly — the ramp is gentle but human HS rates do cluster.
	if raw, n, ok := channelRaw(ps, channelCategoryKills, Key("headshot_percentage"), channelCategoryKills, Key("total_kills")); ok && n >= 10 {
		if tier := narrativeTier(raw, 55.0, 65.0, 75.0, true); tier > 0 {
			out = append(out, narrativeChannel{id: "hs", tier: tier, raw: raw, sampleN: n})
		}
	}

	// Snap P95 velocity: ascending suspicion. The channel ramp is 2.0→3.5 °/ms,
	// but in practice most riflers cross 2 occasionally — meaningful outliers
	// start around 6, and the wingman cheaters logged ~8.
	if raw, n, ok := channelRaw(ps, channelCategoryAiming, Key("p95_snap_velocity"), channelCategoryAiming, Key("snap_count")); ok && n >= 5 {
		if tier := narrativeTier(raw, 4.0, 6.0, 10.0, true); tier > 0 {
			out = append(out, narrativeChannel{id: "snap", tier: tier, raw: raw, sampleN: n})
		}
	}

	// Median time-to-damage: descending suspicion (lower = consistently fast =
	// more suspect). Anchored on docs/METRICS.md — Leetify Public Data Library:
	// clean 500–600ms (Premier 5–15k baseline), suspicious <250ms, blatant
	// <150ms. Median is the "well-established" signal; P10 alone is too noisy
	// because a single pre-fired tick produces sub-100ms reactions
	// legitimately. Median requires a sustained fast-reaction pattern.
	if raw, n, ok := channelRaw(ps, channelCategoryReaction, Key("median_ttd"), channelCategoryReaction, Key("ttd_samples")); ok && n >= 10 {
		if tier := narrativeTier(raw, 400.0, 250.0, 150.0, false); tier > 0 {
			out = append(out, narrativeChannel{id: "reaction", tier: tier, raw: raw, sampleN: n})
		}
	}

	// Sub-100ms TTD rate: ascending. Any sustained sub-100ms response rate is
	// suspicious; 30%+ is the "triggerbot pattern" zone.
	if raw, n, ok := channelRaw(ps, channelCategoryReaction, Key("sub_100ms_ttd"), channelCategoryReaction, Key("ttd_samples")); ok && n >= 5 {
		if tier := narrativeTier(raw, 15.0, 25.0, 35.0, true); tier > 0 {
			out = append(out, narrativeChannel{id: "ttd_sub100", tier: tier, raw: raw, sampleN: n})
		}
	}

	// Recoil score: ascending, but score-based rather than raw-meaningful.
	// The published recoil_score is already 0-1.
	if raw, n, ok := channelRaw(ps, channelCategoryRecoil, Key("recoil_score"), channelCategoryRecoil, Key("total_counted_bullets")); ok && n >= 20 {
		if tier := narrativeTier(raw, 0.30, 0.55, 0.80, true); tier > 0 {
			out = append(out, narrativeChannel{id: "recoil", tier: tier, raw: raw, sampleN: n})
		}
	}

	// Pre-FOV pre-aim median: descending (tighter = more suspect). Wingman
	// cheaters land 6.17° and 7.25°; szpont is at 5.85°. Pros typically 7-10°.
	if raw, n, ok := channelRaw(ps, channelCategoryBehavioral, Key("pre_fov_aim_median_deg"), channelCategoryBehavioral, Key("pre_fov_aim_samples")); ok && n >= 4 {
		if tier := narrativeTier(raw, 8.0, 7.0, 6.0, false); tier > 0 {
			out = append(out, narrativeChannel{id: "pre_fov", tier: tier, raw: raw, sampleN: n})
		}
	}

	// Pre-FOV presence: fires only when the boost rule fires (lobby asymmetry
	// + tight median). Use the published *_score to detect that it fired.
	if score, ok := psGetFloat(ps, cheatscoreCategoryAntiCheat, Key("pre_fov_presence_score")); ok && score >= 0.50 {
		n, _ := psGetInt(ps, channelCategoryBehavioral, Key("pre_fov_aim_samples"))
		tier := 2
		if score >= 0.80 {
			tier = 3
		}
		out = append(out, narrativeChannel{id: "pre_fov_presence", tier: tier, raw: score, sampleN: n})
	}

	// Off-engagement attention drift: descending (smaller drift to nearest
	// enemy = more aware = more suspect). Pro baseline ~30°; suspect <25°.
	if raw, n, ok := channelRaw(ps, channelCategoryBehavioral, Key("nearest_enemy_angle_median_deg"), channelCategoryBehavioral, Key("nearest_enemy_angle_samples")); ok && n >= 200 {
		if tier := narrativeTier(raw, 27.0, 24.0, 20.0, false); tier > 0 {
			out = append(out, narrativeChannel{id: "attention", tier: tier, raw: raw, sampleN: n})
		}
	}

	// Back-killed rate: descending (lower = never caught from behind = more
	// suspect). Need ≥8 deaths to draw any conclusion; 0% on a large sample
	// is the strongest reading.
	if raw, n, ok := channelRaw(ps, channelCategoryBehavioral, Key("back_killed_pct"), channelCategoryBehavioral, Key("back_killed_total_deaths")); ok && n >= 8 {
		if tier := narrativeTier(raw, 8.0, 4.0, 0.01, false); tier > 0 {
			out = append(out, narrativeChannel{id: "back_killed", tier: tier, raw: raw, sampleN: n})
		}
	}

	// Decoupling: use the published score directly (the channel raw is a
	// derived delta that's less intuitive in prose).
	if score, ok := psGetFloat(ps, cheatscoreCategoryAntiCheat, Key("decoupling_score")); ok {
		conf, _ := psGetFloat(ps, cheatscoreCategoryAntiCheat, Key("decoupling_confidence"))
		prod := score * conf
		var tier int
		switch {
		case prod >= 0.60:
			tier = 3
		case prod >= 0.45:
			tier = 2
		case prod >= 0.30:
			tier = 1
		}
		if tier > 0 {
			out = append(out, narrativeChannel{id: "decoupling", tier: tier, raw: score})
		}
	}

	return out
}

func narrativeSentence(c narrativeChannel, adj string) string {
	switch c.id {
	case "hs":
		return fmt.Sprintf("Headshot rate of %.0f%% over %d kills is %s elevated.", c.raw, c.sampleN, adj)
	case "snap":
		return fmt.Sprintf("P95 snap velocity of %.2f °/ms across %d snaps is %s above the lobby baseline.", c.raw, c.sampleN, adj)
	case "reaction":
		return fmt.Sprintf("Median time-to-damage of %.0f ms across %d engagements is %s fast — consistently quick reactions rather than a single prefired tick.", c.raw, c.sampleN, adj)
	case "ttd_sub100":
		return fmt.Sprintf("Sub-100 ms time-to-damage rate of %.1f%% across %d samples is %s implausible without information assistance.", c.raw, c.sampleN, adj)
	case "recoil":
		return fmt.Sprintf("Recoil-control anomaly is %s elevated.", adj)
	case "pre_fov":
		return fmt.Sprintf("Pre-FOV pre-aim median of %.2f° on %d kills is %s tight — crosshair near enemy positions before line-of-sight.", c.raw, c.sampleN, adj)
	case "pre_fov_presence":
		return fmt.Sprintf("Pre-FOV pre-aim presence fires on %d samples — lobby asymmetry indicates other players don't produce the same pre-aim pattern.", c.sampleN)
	case "attention":
		return fmt.Sprintf("Off-engagement crosshair drifts to within a %.1f° median of the nearest enemy — %s aware of unseen opponents.", c.raw, adj)
	case "back_killed":
		return fmt.Sprintf("Back-killed rate of %.1f%% across %d deaths is %s low — rarely caught from behind.", c.raw, c.sampleN, adj)
	case "decoupling":
		return fmt.Sprintf("Fight-vs-idle attention split is %s decoupled — focused during engagements, drifting toward unseen enemies between them.", adj)
	}
	return ""
}

func psHasYes(ps *PlayerStats, k Key) bool {
	m, ok := ps.GetMetric(cheatscoreCategoryAntiCheat, k)
	return ok && m.StringValue == "Yes"
}
