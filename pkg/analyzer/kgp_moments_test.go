package analyzer

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
)

// TestDetector_KGPSzpontMoments dumps a per-moment aim-vs-LOS table for szpont
// at the three suspicious timestamps the user flagged in the KGP vs Crystal
// Club demo:
//
//	3-2 at 0:51 remaining (round 6, ~64s in)
//	8-8 at 1:41 remaining (round 17, ~14s in)
//	10-12 at 1:08 remaining (round 23, ~47s in)
//
// For each target we dump a ±5s window. Every ~250ms we log szpont's view
// angle, the nearest enemy to his crosshair, that enemy's spotted-by-szpont
// status, the angle delta, and whether he fired/killed on that tick. The
// wallhack signature is a low angle-delta to an UNSPOTTED enemy sustained
// across multiple samples — info-without-LOS.
//
// Diagnostic only — does not assert anything. Run with:
//
//	go test ./pkg/analyzer/ -run TestDetector_KGPSzpontMoments -v
func TestDetector_KGPSzpontMoments(t *testing.T) {
	const (
		szpontID    = uint64(76561199039310400)
		roundLenSec = 115.0 // CS2 competitive round length
		windowSec   = 5.0   // ± window around target
		sampleStep  = 16    // ticks between samples (~250ms at 64 tickrate)
	)

	abs, err := filepath.Abs(kgpDemoPath)
	if err != nil {
		t.Fatalf("resolve %s: %v", kgpDemoPath, err)
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		t.Skipf("KGP demo not present at %s", abs)
	}

	type target struct {
		label       string
		round       int
		elapsedAtSec float64
	}
	// "3-2 — 0:51" → round 6, 115-51=64s in
	// "8-8 — 1:41" → round 17, 115-101=14s in
	// "10-12 — 1:08" → round 23, 115-68=47s in
	targets := []target{
		{label: "3-2 @ 0:51 remaining (round 6, ~64s in)", round: 6, elapsedAtSec: 64},
		{label: "8-8 @ 1:41 remaining (round 17, ~14s in)", round: 17, elapsedAtSec: 14},
		{label: "10-12 @ 1:08 remaining (round 23, ~47s in)", round: 23, elapsedAtSec: 47},
	}

	type sample struct {
		round            int
		elapsedSec       float64
		szpontEyeX, szpontEyeY, szpontEyeZ float64
		yaw, pitch       float64
		// nearest unspotted enemy stats:
		nearestUnspottedName  string
		nearestUnspottedAngle float64
		nearestUnspottedDist  float64
		nearestUnspottedAlive bool
		// nearest spotted enemy stats (whatever szpont can see):
		nearestSpottedName  string
		nearestSpottedAngle float64
		// kill / fire event flags
		fired  bool
		killed string
	}

	// Bucket: target index → samples. Aligned with `targets` slice.
	collected := make([][]sample, len(targets))

	f, err := os.Open(abs)
	if err != nil {
		t.Fatalf("open demo: %v", err)
	}
	defer f.Close()

	parser := demoinfocs.NewParser(f)
	defer parser.Close()

	var (
		currentRound      = 0
		roundStartTick    = 0
		tickRate          = 64.0
		recentFireBy      = map[uint64]int{} // sid → tick of last shot
		recentKillBy      = map[uint64]string{}
	)

	parser.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		currentRound++
		roundStartTick = parser.GameState().IngameTick()
	})

	parser.RegisterEventHandler(func(e events.WeaponFire) {
		if e.Shooter != nil && e.Shooter.SteamID64 == szpontID {
			recentFireBy[szpontID] = parser.GameState().IngameTick()
		}
	})

	parser.RegisterEventHandler(func(e events.Kill) {
		if e.Killer != nil && e.Killer.SteamID64 == szpontID && e.Victim != nil {
			recentKillBy[szpontID] = e.Victim.Name + " (by szpont)"
		}
		if e.Victim != nil && e.Victim.SteamID64 == szpontID && e.Killer != nil {
			recentKillBy[szpontID] = "DIED to " + e.Killer.Name
		}
	})

	if rate := parser.TickRate(); rate > 0 {
		tickRate = rate
	}
	parser.RegisterEventHandler(func(e events.TickRateInfoAvailable) {
		if e.TickRate > 0 {
			tickRate = e.TickRate
		}
	})

	// Frame loop.
	for {
		more, err := parser.ParseNextFrame()
		if err != nil || !more {
			break
		}
		gs := parser.GameState()
		curTick := gs.IngameTick()

		if currentRound == 0 || roundStartTick == 0 {
			continue
		}
		elapsedSec := float64(curTick-roundStartTick) / tickRate

		// Which target are we in?
		targetIdx := -1
		for i, tgt := range targets {
			if currentRound != tgt.round {
				continue
			}
			if math.Abs(elapsedSec-tgt.elapsedAtSec) <= windowSec {
				targetIdx = i
				break
			}
		}
		if targetIdx < 0 {
			continue
		}

		// Sub-sample so we don't dump every tick.
		if (curTick-roundStartTick)%sampleStep != 0 {
			continue
		}

		// Find szpont.
		var szpont *common.Player
		for _, p := range gs.Participants().Playing() {
			if p != nil && p.SteamID64 == szpontID {
				szpont = p
				break
			}
		}
		if szpont == nil || !szpont.IsAlive() {
			continue
		}

		eye := szpont.Position()
		// EyePosition includes view offset; preferred for LOS reasoning.
		if ep, ok := szpont.PositionEyes(); ok && (ep.Z != 0 || ep.X != 0) {
			eye = ep
		}
		yaw := float64(szpont.ViewDirectionX())
		pitch := float64(szpont.ViewDirectionY())
		viewVec := viewDirToVec(yaw, pitch)

		s := sample{
			round:      currentRound,
			elapsedSec: elapsedSec,
			szpontEyeX: eye.X,
			szpontEyeY: eye.Y,
			szpontEyeZ: eye.Z,
			yaw:        yaw,
			pitch:      pitch,
		}

		// Walk enemies, find nearest-by-angle for spotted and unspotted.
		s.nearestUnspottedAngle = 180
		s.nearestSpottedAngle = 180
		for _, opp := range gs.Participants().Playing() {
			if opp == nil || opp.SteamID64 == 0 || opp.SteamID64 == szpontID {
				continue
			}
			if opp.Team == szpont.Team {
				continue
			}
			if !opp.IsAlive() {
				continue
			}
			opos := opp.Position()
			dx := opos.X - eye.X
			dy := opos.Y - eye.Y
			dz := opos.Z - eye.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			mag := dist
			if mag == 0 {
				continue
			}
			dot := (viewVec[0]*dx + viewVec[1]*dy + viewVec[2]*dz) / mag
			if dot > 1 {
				dot = 1
			} else if dot < -1 {
				dot = -1
			}
			ang := math.Acos(dot) * 180.0 / math.Pi

			// Spotted by szpont == server-side LOS yes (engine flag).
			spotted := opp.IsSpottedBy(szpont)

			if spotted {
				if ang < s.nearestSpottedAngle {
					s.nearestSpottedAngle = ang
					s.nearestSpottedName = opp.Name
				}
			} else {
				if ang < s.nearestUnspottedAngle {
					s.nearestUnspottedAngle = ang
					s.nearestUnspottedDist = dist
					s.nearestUnspottedName = opp.Name
					s.nearestUnspottedAlive = true
				}
			}
		}

		// Recent action flags within last 8 ticks (~125 ms).
		if t, ok := recentFireBy[szpontID]; ok && curTick-t < 8 {
			s.fired = true
		}
		if k, ok := recentKillBy[szpontID]; ok {
			s.killed = k
			delete(recentKillBy, szpontID)
		}

		collected[targetIdx] = append(collected[targetIdx], s)
	}

	// Print per target.
	for i, tgt := range targets {
		samples := collected[i]
		t.Logf("\n=== %s ===", tgt.label)
		if len(samples) == 0 {
			t.Logf("  (no samples — szpont possibly dead, or target window outside parsed range)")
			continue
		}
		sort.Slice(samples, func(a, b int) bool { return samples[a].elapsedSec < samples[b].elapsedSec })
		t.Logf("  %-8s %-9s %-22s %-7s %-22s %-7s %-6s %s",
			"elapsed", "view(yaw)", "unspotted_enemy", "Δang°", "spotted_enemy", "Δang°", "fired", "event")
		for _, s := range samples {
			unspot := "—"
			unspotAng := "—"
			if s.nearestUnspottedAlive {
				unspot = fmt.Sprintf("%s(d=%.0f)", s.nearestUnspottedName, s.nearestUnspottedDist)
				unspotAng = fmt.Sprintf("%.1f", s.nearestUnspottedAngle)
			}
			spot := "—"
			spotAng := "—"
			if s.nearestSpottedName != "" {
				spot = s.nearestSpottedName
				spotAng = fmt.Sprintf("%.1f", s.nearestSpottedAngle)
			}
			fired := ""
			if s.fired {
				fired = "FIRE"
			}
			t.Logf("  %-8.2f %-9.1f %-22s %-7s %-22s %-7s %-6s %s",
				s.elapsedSec, s.yaw, unspot, unspotAng, spot, spotAng, fired, s.killed)
		}
	}
}

func viewDirToVec(yawDeg, pitchDeg float64) [3]float64 {
	if pitchDeg > 180 {
		pitchDeg -= 360
	}
	yaw := yawDeg * math.Pi / 180
	pitch := pitchDeg * math.Pi / 180
	cp := math.Cos(pitch)
	return [3]float64{math.Cos(yaw) * cp, math.Sin(yaw) * cp, math.Sin(pitch)}
}
