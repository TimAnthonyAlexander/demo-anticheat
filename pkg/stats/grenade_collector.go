package stats

import (
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/oklog/ulid/v2"
)

const grenadeCategory = Category("utility")

// GrenadeCollector tracks per-player HE grenade usage and damage. We restrict
// to HE because that's the unit authoritative demo tools report on and the
// metric that carries cheat signal — a player landing every HE on enemies
// (zero zero-damage throws) implies info advantages (wallhacks, prefires).
// Smokes / flashes / decoys are intentional utility plays with no damage
// component and are excluded.
type GrenadeCollector struct {
	*BaseCollector
	roundCount   int
	heExplosions map[ulid.ULID]*heExplosion
}

type heExplosion struct {
	thrower uint64
	damage  int64
	enemies int64
}

func NewGrenadeCollector() *GrenadeCollector {
	return &GrenadeCollector{
		BaseCollector: NewBaseCollector("Grenades", grenadeCategory),
		heExplosions:  map[ulid.ULID]*heExplosion{},
	}
}

func (gc *GrenadeCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	parser.RegisterEventHandler(func(_ events.RoundEnd) {
		gc.roundCount++
	})

	// Each HE detonation is one "thrown" by the thrower. Tracking by Equipment
	// UniqueID2 lets us attribute damage events back to the specific HE.
	parser.RegisterEventHandler(func(e events.HeExplode) {
		if parser.GameState().IsWarmupPeriod() {
			return
		}
		if e.Thrower == nil || e.Grenade == nil {
			return
		}
		gc.heExplosions[e.Grenade.UniqueID2()] = &heExplosion{
			thrower: e.Thrower.SteamID64,
		}
		ps := demoStats.GetOrCreatePlayerStats(e.Thrower)
		if ps == nil {
			return
		}
		ps.IncrementIntMetric(grenadeCategory, Key("thrown"))
	})

	parser.RegisterEventHandler(func(e events.PlayerHurt) {
		if parser.GameState().IsWarmupPeriod() {
			return
		}
		if e.Attacker == nil || e.Player == nil || e.Attacker == e.Player {
			return
		}
		if e.Weapon == nil || e.Weapon.Type != common.EqHE {
			return
		}
		if e.Attacker.Team == e.Player.Team {
			return
		}
		ps := demoStats.GetOrCreatePlayerStats(e.Attacker)
		if ps == nil {
			return
		}
		dmg := int64(e.HealthDamageTaken)
		addIntMetric(ps, grenadeCategory, Key("damage"), dmg)
		ps.IncrementIntMetric(grenadeCategory, Key("enemy_hits"))

		if info, ok := gc.heExplosions[e.Weapon.UniqueID2()]; ok {
			info.damage += dmg
			info.enemies++
		}
	})

	parser.RegisterEventHandler(func(e events.Kill) {
		if parser.GameState().IsWarmupPeriod() {
			return
		}
		if e.Killer == nil || e.Victim == nil || e.Killer == e.Victim {
			return
		}
		if e.Weapon == nil || e.Weapon.Type != common.EqHE {
			return
		}
		if e.Killer.Team == e.Victim.Team {
			return
		}
		ps := demoStats.GetOrCreatePlayerStats(e.Killer)
		if ps == nil {
			return
		}
		ps.IncrementIntMetric(grenadeCategory, Key("killed"))
	})
}

func (gc *GrenadeCollector) CollectFinalStats(demoStats *DemoStats) {
	heZero := map[uint64]int64{}
	for _, info := range gc.heExplosions {
		if info.damage == 0 {
			heZero[info.thrower]++
		}
	}

	for sid, ps := range demoStats.Players {
		if sid == 0 {
			continue
		}
		thrown := intMetric(ps, grenadeCategory, Key("thrown"))
		if thrown == 0 {
			continue
		}
		damage := intMetric(ps, grenadeCategory, Key("damage"))
		hits := intMetric(ps, grenadeCategory, Key("enemy_hits"))

		ps.AddMetric(grenadeCategory, Key("damage_per_throw"), Metric{
			Type:        MetricFloat,
			FloatValue:  float64(damage) / float64(thrown),
			Description: "Avg HE damage per HE thrown",
		})
		ps.AddMetric(grenadeCategory, Key("enemies_per_throw"), Metric{
			Type:        MetricFloat,
			FloatValue:  float64(hits) / float64(thrown),
			Description: "Avg enemy-damage events per HE thrown",
		})
		if gc.roundCount > 0 {
			ps.AddMetric(grenadeCategory, Key("damage_per_round"), Metric{
				Type:        MetricFloat,
				FloatValue:  float64(damage) / float64(gc.roundCount),
				Description: "Avg HE damage per round",
			})
		}

		ps.AddMetric(grenadeCategory, Key("he_zero_damage"), Metric{
			Type:        MetricInteger,
			IntValue:    heZero[sid],
			Description: "HE grenades that dealt zero damage (lineups, prefires, missed reads — normal humans always have some)",
		})
	}
}
