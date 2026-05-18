package stats

import (
	"math"
	"sort"

	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
)

// BehavioralCollector implements three wallhack-targeted information-channel
// signals that complement the existing aim-mechanics collectors:
//
//  1. Back-kill avoidance — wallhackers are rarely killed from behind because
//     they always know where enemies are.
//  2. Pre-FOV pre-aim angle — angle between killer's view direction and
//     victim's position 200 ms before the victim entered killer's FOV. Pros
//     pre-aim static map angles; wallhackers pre-aim moving enemy positions.
//  3. Off-engagement enemy attention — median per-frame angle from view
//     direction to the nearest alive enemy when no enemy is in FOV. Wallhackers'
//     attention drifts toward enemies they can't legally see.
//
// All three metrics are computed without map BSP / line-of-sight data using
// only positional and view-angle information from the demo.

const (
	// fovEntryDegrees is the half-angle (deg) where we consider a target
	// "in FOV" of a player — matches the reaction collector's threshold.
	fovEntryDegrees = 5.0
	// preFOVLookbackMs is how far before FOV-entry we sample the killer's
	// crosshair-to-victim angle.
	preFOVLookbackMs = 200.0
	// behavioralBufferTicks bounds the per-player view+position history.
	// 5 s at 64 tick.
	behavioralBufferTicks = 320
	// backKillThresholdDeg is the angle from victim's view direction to the
	// killer beyond which the kill counts as "from behind".
	backKillThresholdDeg = 100.0
	// minBackKillSamples avoids scoring with too few deaths (Wingman 2v2 rounds).
	minBackKillSamples = 4
	// minPreFOVSamples avoids scoring with too few kills. 3-sample readings
	// produced noisy false positives (a clean teammate landed at 6.94° on
	// exactly 3 samples — visually wallhack-suspicious but not reliable). The
	// confidence weight on the pre_fov channel doesn't fully damp the
	// downstream decoupling channel or the TTD-sub100 co-occurrence floor, so
	// the noise floor is held at 4.
	minPreFOVSamples = 4
	// minAttentionSamples avoids scoring on tiny per-frame samples.
	minAttentionSamples = 200
)

// playerSnapshot captures view direction + eye-level position at a tick.
type playerSnapshot struct {
	tick  int
	yawX  float64 // ViewDirectionX, deg
	pitch float64 // ViewDirectionY, deg
	posX  float64
	posY  float64
	posZ  float64
}

// BehavioralCollector accumulates the three wallhack-targeted metrics.
type BehavioralCollector struct {
	*BaseCollector

	tickRate    float64
	currentTick int

	// Per-player rolling history of view + position.
	history map[uint64][]playerSnapshot

	// Per-player accumulators.
	backKillTotal      map[uint64]int // total deaths charged to this player as victim
	backKillBack       map[uint64]int // deaths where victim was looking away from killer
	backKillGivenTotal map[uint64]int // total kills charged to this player as killer
	backKillGivenBack  map[uint64]int // kills where victim was looking away from this killer
	preFOVAngles       map[uint64][]float64
	attentionMin       map[uint64][]float64
}

// NewBehavioralCollector creates a new BehavioralCollector.
func NewBehavioralCollector() *BehavioralCollector {
	return &BehavioralCollector{
		BaseCollector:      NewBaseCollector("Behavioral Wallhack Signals", Category("behavioral")),
		history:            make(map[uint64][]playerSnapshot),
		backKillTotal:      make(map[uint64]int),
		backKillBack:       make(map[uint64]int),
		backKillGivenTotal: make(map[uint64]int),
		backKillGivenBack:  make(map[uint64]int),
		preFOVAngles:       make(map[uint64][]float64),
		attentionMin:       make(map[uint64][]float64),
	}
}

// Setup registers kill handler and seeds the tick rate.
func (bc *BehavioralCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	bc.tickRate = parser.TickRate()
	if bc.tickRate <= 0 {
		bc.tickRate = 64.0
	}
	parser.RegisterEventHandler(func(e events.TickRateInfoAvailable) {
		if e.TickRate > 0 {
			bc.tickRate = e.TickRate
		}
	})

	parser.RegisterEventHandler(func(e events.Kill) {
		bc.handleKill(e)
	})
}

// CollectFrame snapshots state and accumulates the off-engagement attention metric.
func (bc *BehavioralCollector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	bc.currentTick = parser.CurrentFrame()
	gs := parser.GameState()

	playing := gs.Participants().Playing()

	// Snapshot every alive player into rolling history.
	for _, p := range playing {
		if p == nil || p.SteamID64 == 0 || !p.IsAlive() {
			continue
		}
		pos := p.Position()
		snap := playerSnapshot{
			tick:  bc.currentTick,
			yawX:  float64(p.ViewDirectionX()),
			pitch: float64(p.ViewDirectionY()),
			posX:  pos.X,
			posY:  pos.Y,
			posZ:  pos.Z,
		}
		buf := bc.history[p.SteamID64]
		buf = append(buf, snap)
		if len(buf) > behavioralBufferTicks {
			buf = buf[len(buf)-behavioralBufferTicks:]
		}
		bc.history[p.SteamID64] = buf
	}

	// Off-engagement attention: for each alive player, find the smallest
	// angle from view-direction to any alive enemy. Only record samples where
	// no enemy is currently in FOV (>= fovEntryDegrees from the closest one),
	// so we measure attention drift, not active engagements.
	for _, attacker := range playing {
		if attacker == nil || attacker.SteamID64 == 0 || !attacker.IsAlive() {
			continue
		}
		viewVec := viewDirectionToVector(float64(attacker.ViewDirectionX()), float64(attacker.ViewDirectionY()))
		attackerPos := attacker.Position()

		minAngle := 180.0
		for _, opponent := range playing {
			if opponent == nil || opponent.SteamID64 == 0 || !opponent.IsAlive() {
				continue
			}
			if opponent.Team == attacker.Team || opponent.SteamID64 == attacker.SteamID64 {
				continue
			}
			oppPos := opponent.Position()
			ang := angleBetweenViewAndTarget(viewVec, attackerPos.X, attackerPos.Y, attackerPos.Z, oppPos.X, oppPos.Y, oppPos.Z)
			if ang < minAngle {
				minAngle = ang
			}
		}

		// Skip frames where attacker is actively in an engagement; those
		// are the legitimate "look at the enemy you're shooting at" frames.
		if minAngle < fovEntryDegrees {
			continue
		}
		bc.attentionMin[attacker.SteamID64] = append(bc.attentionMin[attacker.SteamID64], minAngle)
	}
}

// handleKill computes back-kill rate and pre-FOV pre-aim angle for the killer.
func (bc *BehavioralCollector) handleKill(e events.Kill) {
	if e.Killer == nil || e.Victim == nil {
		return
	}
	if e.Killer.Team == e.Victim.Team {
		return // ignore team kills
	}
	if e.Killer.SteamID64 == 0 || e.Victim.SteamID64 == 0 {
		return
	}

	killerID := e.Killer.SteamID64
	victimID := e.Victim.SteamID64

	// --- Back-kill metric (charged to both sides) --------------------
	// Was the victim looking away from the killer at the moment of death?
	// The same yes/no answer counts as evidence on opposite directions:
	//   - victim side: high rate = unaware = clean (or info-cheater?)
	//                  low rate = always-aware = wallhack-suspicious
	//   - killer side: high rate = kills mostly from behind = could mean either
	//                  preferred flanking style OR a wallhacker exploiting
	//                  positional info to approach unseen.
	killerPos := e.Killer.Position()
	victimPos := e.Victim.Position()
	victimView := viewDirectionToVector(float64(e.Victim.ViewDirectionX()), float64(e.Victim.ViewDirectionY()))
	angVictimToKiller := angleBetweenViewAndTarget(victimView, victimPos.X, victimPos.Y, victimPos.Z, killerPos.X, killerPos.Y, killerPos.Z)
	bc.backKillTotal[victimID]++
	bc.backKillGivenTotal[killerID]++
	if angVictimToKiller >= backKillThresholdDeg {
		bc.backKillBack[victimID]++
		bc.backKillGivenBack[killerID]++
	}

	// --- Pre-FOV pre-aim metric (charged to the KILLER) -------------
	// Walk the killer's history backward from the kill tick to find when
	// the victim FIRST entered the killer's FOV. Then look further back by
	// preFOVLookbackMs and measure the killer's view-to-victim angle then.
	killerHistory := bc.history[killerID]
	if len(killerHistory) < 2 {
		return
	}

	// Walk back through killer history, computing angle from killer's view
	// to victim's position. Victim's position at each tick is approximated
	// by the victim's history at the same tick if available; otherwise use
	// the kill-time position (small error for short engagements).
	victimHistory := bc.history[victimID]
	victimByTick := make(map[int]playerSnapshot, len(victimHistory))
	for _, s := range victimHistory {
		victimByTick[s.tick] = s
	}

	fovEntryIdx := -1
	for i := len(killerHistory) - 1; i >= 0; i-- {
		ks := killerHistory[i]
		vs, ok := victimByTick[ks.tick]
		if !ok {
			continue
		}
		viewVec := viewDirectionToVector(ks.yawX, ks.pitch)
		ang := angleBetweenViewAndTarget(viewVec, ks.posX, ks.posY, ks.posZ, vs.posX, vs.posY, vs.posZ)
		if ang >= fovEntryDegrees {
			// First tick going backward where victim is OUT of FOV → the
			// frame after this one is the FOV entry from the killer's POV.
			fovEntryIdx = i + 1
			break
		}
	}
	if fovEntryIdx <= 0 || fovEntryIdx >= len(killerHistory) {
		return
	}

	// Look back preFOVLookbackMs from the FOV-entry tick.
	tickRate := bc.tickRate
	if tickRate <= 0 {
		tickRate = 64.0
	}
	lookbackTicks := int(preFOVLookbackMs * tickRate / 1000.0)
	targetTick := killerHistory[fovEntryIdx].tick - lookbackTicks

	// Find the killer/victim snapshots closest to targetTick (still in buffer).
	var ks, vs playerSnapshot
	foundK, foundV := false, false
	for i := fovEntryIdx; i >= 0; i-- {
		if killerHistory[i].tick <= targetTick {
			ks = killerHistory[i]
			foundK = true
			break
		}
	}
	if !foundK {
		return // not enough history
	}
	if v, ok := victimByTick[ks.tick]; ok {
		vs = v
		foundV = true
	}
	if !foundV {
		return
	}

	viewVec := viewDirectionToVector(ks.yawX, ks.pitch)
	preFOVAngle := angleBetweenViewAndTarget(viewVec, ks.posX, ks.posY, ks.posZ, vs.posX, vs.posY, vs.posZ)
	bc.preFOVAngles[killerID] = append(bc.preFOVAngles[killerID], preFOVAngle)
}

// CollectFinalStats publishes the per-player aggregates as metrics.
func (bc *BehavioralCollector) CollectFinalStats(demoStats *DemoStats) {
	for sid, ps := range demoStats.Players {
		// --- Back-kill rate (victim side) ---------------------------
		if total := bc.backKillTotal[sid]; total >= minBackKillSamples {
			back := bc.backKillBack[sid]
			rate := float64(back) / float64(total)
			ps.AddMetric(Category("behavioral"), Key("back_killed_pct"), Metric{
				Type:        MetricPercentage,
				FloatValue:  rate * 100.0,
				Description: "Percent of own deaths where this player was looking away from the killer (low = suspicious)",
			})
			ps.AddMetric(Category("behavioral"), Key("back_killed_total_deaths"), Metric{
				Type:        MetricInteger,
				IntValue:    int64(total),
				Description: "Total deaths used for back-kill rate",
			})
		}

		// --- Back-kill rate (killer side) ---------------------------
		// Diagnostic metric: kills where the victim was looking away from
		// the killer. Not currently fed into the cheat-score combiner —
		// flanking is a legitimate playstyle and the false-positive rate
		// for elevated rates here would dwarf the signal. Tracked so it
		// can be inspected per player.
		if total := bc.backKillGivenTotal[sid]; total >= minBackKillSamples {
			back := bc.backKillGivenBack[sid]
			rate := float64(back) / float64(total)
			ps.AddMetric(Category("behavioral"), Key("back_kill_given_pct"), Metric{
				Type:        MetricPercentage,
				FloatValue:  rate * 100.0,
				Description: "Percent of own kills where the victim was looking away from this player (high = flanking or info exploit)",
			})
			ps.AddMetric(Category("behavioral"), Key("back_kill_given_count"), Metric{
				Type:        MetricInteger,
				IntValue:    int64(back),
				Description: "Number of own kills where the victim was looking away",
			})
			ps.AddMetric(Category("behavioral"), Key("back_kill_given_total_kills"), Metric{
				Type:        MetricInteger,
				IntValue:    int64(total),
				Description: "Total kills used for back-kill-given rate",
			})
		}

		// --- Pre-FOV pre-aim angle ---------------------------------
		if angles := bc.preFOVAngles[sid]; len(angles) >= minPreFOVSamples {
			med := median(angles)
			ps.AddMetric(Category("behavioral"), Key("pre_fov_aim_median_deg"), Metric{
				Type:        MetricFloat,
				FloatValue:  med,
				Description: "Median angle (deg) between killer view and victim position 200 ms before FOV entry (low = suspicious)",
			})
			ps.AddMetric(Category("behavioral"), Key("pre_fov_aim_samples"), Metric{
				Type:        MetricInteger,
				IntValue:    int64(len(angles)),
				Description: "Number of kills contributing to pre-FOV pre-aim metric",
			})
		}

		// --- Off-engagement enemy attention ------------------------
		if angles := bc.attentionMin[sid]; len(angles) >= minAttentionSamples {
			med := median(angles)
			ps.AddMetric(Category("behavioral"), Key("nearest_enemy_angle_median_deg"), Metric{
				Type:        MetricFloat,
				FloatValue:  med,
				Description: "Median per-frame angle (deg) from view direction to nearest enemy when not in FOV (low = suspicious)",
			})
			ps.AddMetric(Category("behavioral"), Key("nearest_enemy_angle_samples"), Metric{
				Type:        MetricInteger,
				IntValue:    int64(len(angles)),
				Description: "Number of frames contributing to nearest-enemy attention metric",
			})
		}
	}
}

// --- math helpers --------------------------------------------------

// viewDirectionToVector converts CS2 yaw/pitch (deg) to a unit direction vector.
//
// CS2's ViewDirectionY is documented as 270..90 (with 270 == -90). Treating
// pitch as a signed angle in [-90, 90] requires normalizing the 270 case.
func viewDirectionToVector(yawDeg, pitchDeg float64) [3]float64 {
	if pitchDeg > 180.0 {
		pitchDeg -= 360.0
	}
	yaw := yawDeg * math.Pi / 180.0
	pitch := pitchDeg * math.Pi / 180.0
	cp := math.Cos(pitch)
	return [3]float64{
		math.Cos(yaw) * cp,
		math.Sin(yaw) * cp,
		math.Sin(pitch),
	}
}

// angleBetweenViewAndTarget returns the angle (deg) between a unit view vector
// and the vector from view origin to the target position.
func angleBetweenViewAndTarget(view [3]float64, ox, oy, oz, tx, ty, tz float64) float64 {
	dx := tx - ox
	dy := ty - oy
	dz := tz - oz
	mag := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if mag == 0 {
		return 0
	}
	dot := (view[0]*dx + view[1]*dy + view[2]*dz) / mag
	if dot > 1 {
		dot = 1
	} else if dot < -1 {
		dot = -1
	}
	return math.Acos(dot) * 180.0 / math.Pi
}

// median returns the median of a non-empty slice (mutates input order).
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := make([]float64, len(xs))
	copy(cp, xs)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2.0
}
