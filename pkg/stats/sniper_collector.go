package stats

import (
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
)

const sniperCategory = Category("sniper")

// SniperCollector tracks sniper-specific kill signals that act as
// high-confidence cheat overrides in the detector:
//
//   - sniper_wallbang_kills: kills using AWP / Scout / Scar-20 / G3SG1
//     where PenetratedObjects > 0 (the bullet went through a wall). A handful
//     across a match is normal pre-fire wallbang play; >10 across a match
//     implies the player consistently knew where enemies were behind walls.
//
//   - scout_kills + scout_hs_rate: SSG-08 hits are 1-shot to the head only at
//     close-medium range. Landing 10+ Scout kills with >=80% HS rate without
//     aim assistance is statistically implausible — Scout's slow rate of fire,
//     no aim punch, and pixel-tight head hitbox don't allow it.
type SniperCollector struct {
	*BaseCollector
}

func NewSniperCollector() *SniperCollector {
	return &SniperCollector{
		BaseCollector: NewBaseCollector("Sniper Kills", sniperCategory),
	}
}

func isSniper(t common.EquipmentType) bool {
	switch t {
	case common.EqAWP, common.EqScout, common.EqScar20, common.EqG3SG1:
		return true
	}
	return false
}

func (sc *SniperCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	parser.RegisterEventHandler(func(e events.Kill) {
		if e.Killer == nil || e.Killer.SteamID64 == 0 || e.Victim == nil {
			return
		}
		if e.Killer == e.Victim || e.Killer.Team == e.Victim.Team {
			return
		}
		if e.Weapon == nil {
			return
		}
		ps := demoStats.GetOrCreatePlayerStats(e.Killer)
		if ps == nil {
			return
		}

		t := e.Weapon.Type
		if isSniper(t) {
			if e.PenetratedObjects > 0 {
				ps.IncrementIntMetric(sniperCategory, Key("sniper_wallbang_kills"))
			}
		}
		if t == common.EqScout {
			ps.IncrementIntMetric(sniperCategory, Key("scout_kills"))
			if e.IsHeadshot {
				ps.IncrementIntMetric(sniperCategory, Key("scout_hs_kills"))
			}
		}
	})
}

func (sc *SniperCollector) CollectFinalStats(demoStats *DemoStats) {
	for sid, ps := range demoStats.Players {
		if sid == 0 {
			continue
		}
		total := intMetric(ps, sniperCategory, Key("scout_kills"))
		if total <= 0 {
			continue
		}
		hs := intMetric(ps, sniperCategory, Key("scout_hs_kills"))
		ps.AddMetric(sniperCategory, Key("scout_hs_rate"), Metric{
			Type:        MetricPercentage,
			FloatValue:  float64(hs) / float64(total) * 100.0,
			Description: "Headshot rate on SSG-08 kills",
		})
	}
}
