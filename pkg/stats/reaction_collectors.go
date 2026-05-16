package stats

import (
	"sort"

	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
)

// ReactionTimeCollector measures Time-To-Damage (TTD): the duration from when
// an enemy first becomes visible to the attacker (CS engine line-of-sight via
// m_bSpottedByMask) to the first damage that attacker deals to them. Matches
// Leetify's definition — composite of reaction + crosshair adjustment + fire
// rate + accuracy. NOT pure cognitive reaction time.
//
// Why LoS, not a cone: a defender holding an angle has the corner inside
// their FOV cone the whole round — but the enemy isn't visible until they
// peek. Cone-based "first sight" fires too early and blows past the TTD cap.
// IsSpottedBy uses the engine's server-side visibility checks, which respect
// walls and geometry.
//
// Expected ranges (per Leetify's published data):
//   - Pro / fast: 400–500 ms
//   - Typical clean players: 500–600 ms
//   - <100 ms in any quantity is statistically implausible without info or aim
//     assistance, since human reaction floor alone is ~150 ms.
//
// Engagements >1000 ms are dropped (trigger-discipline / re-engagement plays).
type ReactionTimeCollector struct {
	*BaseCollector

	// engagements[attackerSID][victimSID] is the current engagement record.
	// An engagement begins when the victim first enters the attacker's FOV
	// cone, persists across brief cone exits (≤ reactionGraceMs), and ends if
	// the victim is out for longer than that — at which point the next entry
	// starts a fresh engagement.
	engagements map[uint64]map[uint64]*engagement

	// ttds[playerSID] = list of TTD samples (in ms).
	ttds map[uint64][]float64

	currentTick int
	tickRate    float64
}

const (
	// reactionMaxEngagementMs caps the time we'll attribute to a single
	// engagement. Leetify uses 1000 ms — beyond that the player most likely
	// disengaged and re-engaged later, which isn't a single "TTD".
	reactionMaxEngagementMs = 1000.0

	// reactionGraceMs is how long LoS can drop before the current engagement
	// ends. Brief visibility flickers between ticks shouldn't reset the entry.
	reactionGraceMs = 200.0

	// reactionMinSamples is the minimum number of TTD samples required to
	// produce stable per-player percentiles. Wingman 2v2 demos run short and
	// produce few engagements per player, so we accept 3 — below that the
	// percentiles aren't meaningful.
	reactionMinSamples = 3
)

// engagement tracks one continuous sighting of a victim by an attacker.
// entryTick is set when the engagement starts; seenTick refreshes every frame
// the victim is in cone. If seenTick falls more than reactionGraceMs behind
// the current tick, the engagement is considered over and the next FOV entry
// starts a new one.
type engagement struct {
	entryTick int
	seenTick  int
	damaged   bool
}

func NewReactionTimeCollector() *ReactionTimeCollector {
	return &ReactionTimeCollector{
		BaseCollector: NewBaseCollector("Reaction Time Analysis", Category("reaction")),
		engagements:   make(map[uint64]map[uint64]*engagement),
		ttds:          make(map[uint64][]float64),
	}
}

func (rtc *ReactionTimeCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	rtc.tickRate = parser.TickRate()
	if rtc.tickRate <= 0 {
		rtc.tickRate = 64.0
	}
	parser.RegisterEventHandler(func(e events.TickRateInfoAvailable) {
		if e.TickRate > 0 {
			rtc.tickRate = e.TickRate
		}
	})

	parser.RegisterEventHandler(func(e events.PlayerHurt) {
		rtc.processDamage(e, demoStats)
	})

	parser.RegisterEventHandler(func(_ events.RoundEnd) {
		rtc.engagements = make(map[uint64]map[uint64]*engagement)
	})

	parser.RegisterEventHandler(func(e events.Kill) {
		if e.Victim != nil {
			rtc.clearForPlayer(e.Victim.SteamID64)
		}
		if e.Killer != nil {
			rtc.clearForPlayer(e.Killer.SteamID64)
		}
	})
}

// processDamage records a TTD sample when the attacker first damages a victim
// during the current engagement (i.e. while that victim is being tracked as
// in-FOV since some entry tick).
func (rtc *ReactionTimeCollector) processDamage(e events.PlayerHurt, demoStats *DemoStats) {
	if e.Attacker == nil || e.Player == nil {
		return
	}
	if e.Attacker.SteamID64 == 0 || e.Player.SteamID64 == 0 {
		return
	}
	if e.Attacker.Team == e.Player.Team {
		return
	}

	attackerID := e.Attacker.SteamID64
	victimID := e.Player.SteamID64

	victims, ok := rtc.engagements[attackerID]
	if !ok {
		return
	}
	eng, ok := victims[victimID]
	if !ok || eng == nil || eng.damaged {
		return
	}

	deltaT := float64(rtc.currentTick-eng.entryTick) * (1000.0 / rtc.tickRate)
	if deltaT < 0 || deltaT > reactionMaxEngagementMs {
		return
	}

	rtc.ttds[attackerID] = append(rtc.ttds[attackerID], deltaT)
	eng.damaged = true
}

func (rtc *ReactionTimeCollector) clearForPlayer(playerID uint64) {
	delete(rtc.engagements, playerID)
	for attackerID, victims := range rtc.engagements {
		delete(victims, playerID)
		rtc.engagements[attackerID] = victims
	}
}

// CollectFrame updates engagement records every tick using CS's server-side
// line-of-sight visibility (IsSpottedBy / m_bSpottedByMask). When LoS is
// first established, an engagement starts and entryTick is recorded. While
// LoS persists, seenTick refreshes. If LoS lapses for longer than the grace
// window, the next visibility starts a fresh engagement.
func (rtc *ReactionTimeCollector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	rtc.currentTick = parser.CurrentFrame()
	gs := parser.GameState()
	graceTicks := int(reactionGraceMs * rtc.tickRate / 1000.0)

	for _, attacker := range gs.Participants().Playing() {
		if attacker == nil || attacker.SteamID64 == 0 || !attacker.IsAlive() {
			continue
		}
		attackerID := attacker.SteamID64
		if _, exists := rtc.engagements[attackerID]; !exists {
			rtc.engagements[attackerID] = make(map[uint64]*engagement)
		}

		for _, opponent := range gs.Participants().Playing() {
			if opponent == nil || opponent.SteamID64 == 0 || opponent.SteamID64 == attackerID {
				continue
			}
			if opponent.Team == attacker.Team || !opponent.IsAlive() {
				continue
			}
			if !opponent.IsSpottedBy(attacker) {
				continue
			}

			eng, tracking := rtc.engagements[attackerID][opponent.SteamID64]
			if !tracking || eng == nil || rtc.currentTick-eng.seenTick > graceTicks {
				rtc.engagements[attackerID][opponent.SteamID64] = &engagement{
					entryTick: rtc.currentTick,
					seenTick:  rtc.currentTick,
				}
			} else {
				eng.seenTick = rtc.currentTick
			}
		}
	}
}

func (rtc *ReactionTimeCollector) CollectFinalStats(demoStats *DemoStats) {
	for playerID, samples := range rtc.ttds {
		if len(samples) < reactionMinSamples {
			continue
		}
		sort.Float64s(samples)

		ps, exists := demoStats.Players[playerID]
		if !exists {
			ps = demoStats.GetOrCreatePlayerStats(&common.Player{
				Name:      "Unknown",
				SteamID64: playerID,
			})
			if ps == nil {
				continue
			}
		}

		median := samples[len(samples)/2]
		p10Idx := int(float64(len(samples)) * 0.1)
		if p10Idx < 0 {
			p10Idx = 0
		}
		p10 := samples[p10Idx]

		sub100 := 0
		for _, t := range samples {
			if t <= 100.0 {
				sub100++
			}
		}
		sub100Ratio := float64(sub100) / float64(len(samples)) * 100.0

		ps.AddMetric(Category("reaction"), Key("median_ttd"), Metric{
			Type:        MetricFloat,
			FloatValue:  median,
			Description: "Median Time-To-Damage in ms (sight → first damage; Leetify-style)",
		})
		ps.AddMetric(Category("reaction"), Key("p10_ttd"), Metric{
			Type:        MetricFloat,
			FloatValue:  p10,
			Description: "10th percentile Time-To-Damage in ms",
		})
		ps.AddMetric(Category("reaction"), Key("sub_100ms_ttd"), Metric{
			Type:        MetricPercentage,
			FloatValue:  sub100Ratio,
			Description: "Share of engagements completed in under 100 ms — statistically implausible without info or aim assistance",
		})
		ps.AddMetric(Category("reaction"), Key("ttd_samples"), Metric{
			Type:        MetricInteger,
			IntValue:    int64(len(samples)),
			Description: "Number of TTD samples collected",
		})

		// Cheat-score component, recalibrated for TTD:
		//   0 at 400 ms (clean), 1 at 100 ms (implausible).
		ttdScore := clamp01((400.0 - p10) / 300.0)
		ps.AddMetric(Category("reaction"), Key("reaction_cheat_score"), Metric{
			Type:        MetricFloat,
			FloatValue:  ttdScore,
			Description: "TTD-derived cheat score (0 at 400 ms P10, 1 at 100 ms P10 or lower)",
		})
	}
}
