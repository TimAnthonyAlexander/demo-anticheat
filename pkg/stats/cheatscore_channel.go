package stats

// cheatscore_channel.go defines the Channel value type and shared helpers
// (confidence, clamp01, zoneFor) used by every cheat-score channel. Scoring
// lives in `cheatscore_*.go` files within the stats package so it can read
// PlayerStats directly without crossing a package boundary; tests target the
// helper functions and channels independently.

import "math"

// Zone is a human-readable band for a channel score, used in the transparency
// reporting layer. The combiner does not consume Zone — it consumes Score.
type Zone uint8

const (
	ZoneNoData Zone = iota
	ZoneClean
	ZoneMild
	ZoneStrong
	ZoneBlatant
)

// String returns the lowercase label published in the anti_cheat metrics.
func (z Zone) String() string {
	switch z {
	case ZoneClean:
		return "clean"
	case ZoneMild:
		return "mild"
	case ZoneStrong:
		return "strong"
	case ZoneBlatant:
		return "blatant"
	default:
		return "no_data"
	}
}

// channelMode controls how a channel's contribution enters the combiner.
//
//   - bidirectional: a "clean" reading produces negative log-odds (genuine
//     evidence the player is not cheating). Use for graded metrics like HS%
//     or P10 TTD where below-clean values legitimately exonerate.
//   - positiveOnly: a "clean" reading contributes 0; only suspicious readings
//     shift the score upward. Use for detector-style metrics like snap
//     velocity or recoil MAE — a clean snap doesn't prove anything.
type channelMode uint8

const (
	bidirectional channelMode = iota
	positiveOnly
)

// Channel is one evaluated anti-cheat signal for one player. All fields are
// derived from PlayerStats; Channel has no behavior of its own beyond carrying
// values into the combiner and the publisher.
type Channel struct {
	ID         string
	Score      float64 // [0, 1] — 0 clean, 1 blatant
	Confidence float64 // [0, 1] — 0 no data, 1 fully trustworthy
	Raw        float64 // underlying measurement (for publish transparency)
	SampleN    int64
	Weight     float64
	Zone       Zone
	Mode       channelMode
	HasData    bool
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// linearScore maps a raw measurement onto [0, 1] using a linear ramp anchored
// at cleanX (=0) and blatantX (=1). Works for both directions — pass
// cleanX < blatantX for "higher is worse" metrics, cleanX > blatantX for
// "lower is worse" metrics.
func linearScore(raw, cleanX, blatantX float64) float64 {
	if cleanX == blatantX {
		return 0
	}
	return clamp01((raw - cleanX) / (blatantX - cleanX))
}

func linearConfidence(n int64, nFull int) float64 {
	if nFull <= 0 || n <= 0 {
		return 0
	}
	return clamp01(float64(n) / float64(nFull))
}

// sqrtConfidence returns sqrt(n / n_full) clamped to [0, 1]. Used for heavy-
// tailed signals where a small number of samples already carries meaningful
// evidence (e.g. sub-100ms TTD events).
func sqrtConfidence(n int64, nFull int) float64 {
	if nFull <= 0 || n <= 0 {
		return 0
	}
	return clamp01(math.Sqrt(float64(n) / float64(nFull)))
}

func zoneFor(score float64) Zone {
	switch {
	case score < 0.25:
		return ZoneClean
	case score < 0.50:
		return ZoneMild
	case score < 0.80:
		return ZoneStrong
	default:
		return ZoneBlatant
	}
}

// --- PlayerStats metric helpers ------------------------------------------

func psGetFloat(ps *PlayerStats, cat Category, key Key) (float64, bool) {
	if m, ok := ps.GetMetric(cat, key); ok {
		return m.FloatValue, true
	}
	return 0, false
}

func psGetInt(ps *PlayerStats, cat Category, key Key) (int64, bool) {
	if m, ok := ps.GetMetric(cat, key); ok {
		return m.IntValue, true
	}
	return 0, false
}

func psGetString(ps *PlayerStats, cat Category, key Key) (string, bool) {
	if m, ok := ps.GetMetric(cat, key); ok {
		return m.StringValue, true
	}
	return "", false
}
