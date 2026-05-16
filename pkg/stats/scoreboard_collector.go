package stats

import (
	"sort"

	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
)

// ScoreboardCollector emits CS2-scoreboard-style basic stats per player:
// team side, kills, deaths, assists, MVPs, damage → ADR, headshot %.
// Also snapshots per-round kill counts so a position_factor (avg rank-fraction
// within team at round 5 / halftime / end) can be derived for the detector.
type ScoreboardCollector struct {
	*BaseCollector
	roundCount int
	snapshots  []map[uint64]playerSnap
	// roundKills counts each player's kills inside the current round. CS2
	// demos don't fire the legacy round_mvp event, so we award MVP to the
	// top fragger of each round at RoundEnd. This misses bomb-planter /
	// defuser MVPs but matches the in-game MVP awarded the vast majority
	// of rounds.
	roundKills map[uint64]int
}

type playerSnap struct {
	kills int64
	side  common.Team
}

const scoreboardCategory = Category("scoreboard")

func NewScoreboardCollector() *ScoreboardCollector {
	return &ScoreboardCollector{
		BaseCollector: NewBaseCollector("Scoreboard", scoreboardCategory),
		roundKills:    map[uint64]int{},
	}
}

func (sc *ScoreboardCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	parser.RegisterEventHandler(func(_ events.RoundStart) {
		// Reset per-round MVP-tracking. We do NOT clear at RoundEnd because
		// RoundEnd fires first, then we award MVP, then the next RoundStart
		// resets.
		sc.roundKills = map[uint64]int{}
	})

	parser.RegisterEventHandler(func(_ events.RoundEnd) {
		sc.roundCount++

		// Award MVP heuristically to the top fragger of this round.
		// Ties broken by lower SteamID (stable).
		var mvpSID uint64
		mvpKills := 0
		for sid, k := range sc.roundKills {
			if k > mvpKills || (k == mvpKills && (mvpSID == 0 || sid < mvpSID)) {
				mvpSID = sid
				mvpKills = k
			}
		}
		if mvpSID != 0 && mvpKills > 0 {
			if ps, ok := demoStats.Players[mvpSID]; ok {
				ps.IncrementIntMetric(scoreboardCategory, Key("mvps"))
			}
		}

		snap := map[uint64]playerSnap{}
		for _, p := range parser.GameState().Participants().Playing() {
			if p == nil || p.SteamID64 == 0 {
				continue
			}
			ps := demoStats.GetOrCreatePlayerStats(p)
			if ps == nil {
				continue
			}
			snap[p.SteamID64] = playerSnap{
				kills: intMetric(ps, scoreboardCategory, Key("kills")),
				side:  p.Team,
			}
		}
		sc.snapshots = append(sc.snapshots, snap)
	})

	parser.RegisterEventHandler(func(e events.Kill) {
		if e.Victim != nil {
			if vps := demoStats.GetOrCreatePlayerStats(e.Victim); vps != nil {
				vps.IncrementIntMetric(scoreboardCategory, Key("deaths"))
				recordTeam(vps, e.Victim)
			}
		}

		teamKill := e.Killer != nil && e.Victim != nil && e.Killer.Team == e.Victim.Team
		if e.Killer != nil && e.Killer != e.Victim && !teamKill {
			if kps := demoStats.GetOrCreatePlayerStats(e.Killer); kps != nil {
				kps.IncrementIntMetric(scoreboardCategory, Key("kills"))
				if e.IsHeadshot {
					kps.IncrementIntMetric(scoreboardCategory, Key("hs_kills"))
				}
				recordTeam(kps, e.Killer)
				sc.roundKills[e.Killer.SteamID64]++
			}
		}

		if e.Assister != nil && e.Assister != e.Killer && e.Assister != e.Victim {
			if aps := demoStats.GetOrCreatePlayerStats(e.Assister); aps != nil {
				aps.IncrementIntMetric(scoreboardCategory, Key("assists"))
				recordTeam(aps, e.Assister)
			}
		}
	})

	parser.RegisterEventHandler(func(e events.PlayerHurt) {
		if e.Attacker == nil || e.Player == nil || e.Attacker == e.Player {
			return
		}
		if e.Attacker.Team == e.Player.Team {
			return
		}
		aps := demoStats.GetOrCreatePlayerStats(e.Attacker)
		if aps == nil {
			return
		}
		addIntMetric(aps, scoreboardCategory, Key("damage"), int64(e.HealthDamageTaken))
		recordTeam(aps, e.Attacker)
	})

	// NOTE: events.RoundMVPAnnouncement is a CS:GO legacy game-event that CS2
	// demos do not emit. MVP counts are computed heuristically at RoundEnd
	// from the per-round top-fragger above. See sc.roundKills.
}

func (sc *ScoreboardCollector) CollectFinalStats(demoStats *DemoStats) {
	for _, ps := range demoStats.Players {
		if sc.roundCount > 0 {
			if dmg, ok := ps.GetMetric(scoreboardCategory, Key("damage")); ok {
				ps.AddMetric(scoreboardCategory, Key("adr"), Metric{
					Type:        MetricFloat,
					FloatValue:  float64(dmg.IntValue) / float64(sc.roundCount),
					Description: "Average damage per round",
				})
			}
		}
		kills, _ := ps.GetMetric(scoreboardCategory, Key("kills"))
		hsKills, _ := ps.GetMetric(scoreboardCategory, Key("hs_kills"))
		if kills.IntValue > 0 {
			ps.AddMetric(scoreboardCategory, Key("hs_percentage"), Metric{
				Type:        MetricPercentage,
				FloatValue:  float64(hsKills.IntValue) / float64(kills.IntValue) * 100,
				Description: "Headshot percentage on the scoreboard",
			})
		}
	}

	sc.assignPositionFactors(demoStats)
}

// assignPositionFactors writes a per-player position_factor metric: the
// average rank-fraction within their current side at round 5, halftime, and
// the final round (0.0 = consistent top, 1.0 = consistent bottom). The cheat
// detector reads this and damps likelihood for players who never climbed off
// the bottom of their team's scoreboard.
func (sc *ScoreboardCollector) assignPositionFactors(demoStats *DemoStats) {
	if len(sc.snapshots) == 0 {
		return
	}
	checkpoints := checkpointIndices(len(sc.snapshots))
	if len(checkpoints) == 0 {
		return
	}

	totals := map[uint64]float64{}
	counts := map[uint64]int{}

	for _, idx := range checkpoints {
		snap := sc.snapshots[idx]

		bySide := map[common.Team][]uint64{}
		for sid, s := range snap {
			if s.side == common.TeamTerrorists || s.side == common.TeamCounterTerrorists {
				bySide[s.side] = append(bySide[s.side], sid)
			}
		}

		for _, sids := range bySide {
			if len(sids) < 2 {
				continue
			}
			sort.SliceStable(sids, func(i, j int) bool {
				return snap[sids[i]].kills > snap[sids[j]].kills
			})
			for rank, sid := range sids {
				frac := float64(rank) / float64(len(sids)-1)
				totals[sid] += frac
				counts[sid]++
			}
		}
	}

	for sid, total := range totals {
		c := counts[sid]
		if c == 0 {
			continue
		}
		ps, ok := demoStats.Players[sid]
		if !ok {
			continue
		}
		ps.AddMetric(scoreboardCategory, Key("position_factor"), Metric{
			Type:        MetricFloat,
			FloatValue:  total / float64(c),
			Description: "Avg rank within team at round 5 / halftime / end (0 = top, 1 = bottom)",
		})
	}
}

// checkpointIndices returns the 0-indexed positions in the snapshot slice for
// round 5, the midpoint, and the final round. Duplicates are deduped (e.g. a
// 10-round demo has midpoint at round 5).
func checkpointIndices(n int) []int {
	rounds := []int{5, n / 2, n}
	seen := map[int]bool{}
	out := []int{}
	for _, r := range rounds {
		if r < 1 || r > n {
			continue
		}
		idx := r - 1
		if seen[idx] {
			continue
		}
		seen[idx] = true
		out = append(out, idx)
	}
	return out
}

func recordTeam(ps *PlayerStats, p *common.Player) {
	label := teamLabel(p.Team)
	if label == "" {
		return
	}
	ps.AddMetric(scoreboardCategory, Key("team"), Metric{
		Type:        MetricString,
		StringValue: label,
		Description: "Most recent team side",
	})
}

func teamLabel(t common.Team) string {
	switch t {
	case common.TeamTerrorists:
		return "T"
	case common.TeamCounterTerrorists:
		return "CT"
	default:
		return ""
	}
}

func addIntMetric(ps *PlayerStats, cat Category, k Key, n int64) {
	existing, _ := ps.GetMetric(cat, k)
	existing.IntValue += n
	existing.Type = MetricInteger
	ps.AddMetric(cat, k, existing)
}
