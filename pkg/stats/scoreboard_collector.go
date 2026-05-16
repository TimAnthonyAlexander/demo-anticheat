package stats

import (
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
)

// ScoreboardCollector emits CS2-scoreboard-style basic stats per player:
// team side, kills, deaths, assists, MVPs, damage → ADR, headshot %.
// Lives in its own category so it stays separate from analytical metrics.
type ScoreboardCollector struct {
	*BaseCollector
	roundCount int
}

const scoreboardCategory = Category("scoreboard")

func NewScoreboardCollector() *ScoreboardCollector {
	return &ScoreboardCollector{
		BaseCollector: NewBaseCollector("Scoreboard", scoreboardCategory),
	}
}

func (sc *ScoreboardCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	parser.RegisterEventHandler(func(_ events.RoundEnd) {
		sc.roundCount++
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

	parser.RegisterEventHandler(func(e events.RoundMVPAnnouncement) {
		if e.Player == nil {
			return
		}
		if ps := demoStats.GetOrCreatePlayerStats(e.Player); ps != nil {
			ps.IncrementIntMetric(scoreboardCategory, Key("mvps"))
			recordTeam(ps, e.Player)
		}
	})
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
