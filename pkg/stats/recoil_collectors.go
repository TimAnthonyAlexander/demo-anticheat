package stats

import (
	"fmt"
	"math"

	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

const (
	// RadToDeg converts radians to degrees
	RecoilRadToDeg = 57.2958
)

// RecoilControlCollector tracks recoil control efficiency to detect no-recoil scripts
type RecoilControlCollector struct {
	*BaseCollector
	sprayStates      map[uint64]*sprayState
	tickRate         float64
	maxBurstGap      int
	minBurstSize     int
	maxBulletIdx     int
	goodThreshold    float64
	perfectThreshold float64
	debugMode        bool // Enable debugging
}

// sprayState tracks the state of a player's weapon spray
type sprayState struct {
	inBurst        bool
	firstTick      int
	firstYaw       float64
	firstPitch     float64
	bulletIndex    int
	lastFireTick   int
	weapon         common.EquipmentType
	sumError       float64
	countedBullets int
}

// NewRecoilControlCollector creates a new RecoilControlCollector
func NewRecoilControlCollector() *RecoilControlCollector {
	return &RecoilControlCollector{
		BaseCollector:    NewBaseCollector("Recoil Control", Category("recoil")),
		sprayStates:      make(map[uint64]*sprayState),
		maxBurstGap:      6,    // Ticks between shots to consider it part of the same burst
		minBurstSize:     4,    // Minimum bullets to consider a valid burst
		maxBulletIdx:     30,   // Maximum bullets to track in a spray pattern
		goodThreshold:    0.7,  // Threshold for good recoil control (in degrees)
		perfectThreshold: 0.3,  // Threshold for suspiciously perfect recoil control (in degrees)
		debugMode:        true, // Set to false in production
	}
}

// Setup registers event handlers for weapon fire events
func (rc *RecoilControlCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	rc.tickRate = parser.TickRate()

	// Register weapon fire event handler
	parser.RegisterEventHandler(func(e events.WeaponFire) {
		rc.handleWeaponFire(e, parser, demoStats)
	})

	// Register player death event to reset burst state
	parser.RegisterEventHandler(func(e events.Kill) {
		if e.Victim != nil && e.Victim.SteamID64 != 0 {
			delete(rc.sprayStates, e.Victim.SteamID64)
		}
	})

	// Register round end event to reset all burst states
	parser.RegisterEventHandler(func(e events.RoundEnd) {
		rc.sprayStates = make(map[uint64]*sprayState)
	})
}

// handleWeaponFire processes weapon fire events
func (rc *RecoilControlCollector) handleWeaponFire(e events.WeaponFire, parser demoinfocs.Parser, demoStats *DemoStats) {
	shooter := e.Shooter
	if shooter == nil || shooter.SteamID64 == 0 {
		return
	}

	// Get current tick
	currentTick := parser.CurrentFrame()
	weapon := e.Weapon

	// Skip non-automatic weapons
	if !isAutomaticWeapon(weapon) {
		return
	}

	steamID := shooter.SteamID64
	state, exists := rc.sprayStates[steamID]

	// If player has no spray state or we need to start a new burst
	if !exists {
		rc.sprayStates[steamID] = &sprayState{
			inBurst:      true,
			firstTick:    currentTick,
			firstYaw:     float64(shooter.ViewDirectionX()), // Already in radians
			firstPitch:   float64(shooter.ViewDirectionY()), // Already in radians
			bulletIndex:  1,
			lastFireTick: currentTick,
			weapon:       weapon.Type,
		}
		return // First shot of a burst, no analysis needed
	}

	if exists && state.inBurst {
		// Continue existing burst if within gap threshold
		if currentTick-state.lastFireTick <= rc.maxBurstGap {
			// Update bullet index first
			state.bulletIndex++

			// Check if the bullet is in the range we want to analyze (4-30)
			if state.bulletIndex >= 4 && state.bulletIndex <= rc.maxBulletIdx {
				// Get the expected recoil offsets for this bullet index (in degrees)
				expectedYawOffset, expectedPitchOffset := getRecoilOffsets(state.weapon, state.bulletIndex)

				// Convert expected offsets to radians since view angles are in radians
				expectedYawOffsetRad := expectedYawOffset / RecoilRadToDeg
				expectedPitchOffsetRad := expectedPitchOffset / RecoilRadToDeg

				// Calculate expected aim angles (initial aim minus the spray pattern offsets)
				expectedYaw := state.firstYaw - expectedYawOffsetRad
				expectedPitch := state.firstPitch - expectedPitchOffsetRad

				// Get actual aim angles at the current tick (also in radians)
				actualYaw := float64(shooter.ViewDirectionX())
				actualPitch := float64(shooter.ViewDirectionY())

				// Calculate angular error (in radians)
				yawDiff := expectedYaw - actualYaw
				pitchDiff := expectedPitch - actualPitch
				angularErrorRad := math.Sqrt(yawDiff*yawDiff + pitchDiff*pitchDiff)

				// Convert to degrees for easier understanding
				angularErrorDeg := angularErrorRad * RecoilRadToDeg

				// Add to player's accumulated error (in degrees)
				state.sumError += angularErrorDeg
				state.countedBullets++

				// For debugging - log the first few players we encounter
				if rc.debugMode && demoStats != nil && steamID%10 == 1 && state.bulletIndex <= 10 {
					playerStats := demoStats.GetOrCreatePlayerStatsBySteamID(steamID)
					if playerStats != nil {
						debugInfo := fmt.Sprintf("Bullet:%d Yaw(exp/act):%.2f/%.2f Pitch(exp/act):%.2f/%.2f ErrDeg:%.2f",
							state.bulletIndex,
							expectedYaw*RecoilRadToDeg,
							actualYaw*RecoilRadToDeg,
							expectedPitch*RecoilRadToDeg,
							actualPitch*RecoilRadToDeg,
							angularErrorDeg)

						key := Key(fmt.Sprintf("debug_bullet_%d", state.bulletIndex))
						playerStats.AddMetric(Category("recoil_debug"), key, Metric{
							Type:        MetricString,
							StringValue: debugInfo,
							Description: "Debug recoil calculation info",
						})
					}
				}
			}

			// Update last fire tick
			state.lastFireTick = currentTick
		} else {
			// Gap too large, end previous burst and start a new one
			rc.finalizeBurst(state, steamID, demoStats)
			rc.sprayStates[steamID] = &sprayState{
				inBurst:      true,
				firstTick:    currentTick,
				firstYaw:     float64(shooter.ViewDirectionX()),
				firstPitch:   float64(shooter.ViewDirectionY()),
				bulletIndex:  1,
				lastFireTick: currentTick,
				weapon:       weapon.Type,
			}
		}
	} else {
		// Start a new burst if not in one
		rc.sprayStates[steamID] = &sprayState{
			inBurst:      true,
			firstTick:    currentTick,
			firstYaw:     float64(shooter.ViewDirectionX()),
			firstPitch:   float64(shooter.ViewDirectionY()),
			bulletIndex:  1,
			lastFireTick: currentTick,
			weapon:       weapon.Type,
		}
	}
}

// finalizeBurst processes the end of a burst and calculates statistics
func (rc *RecoilControlCollector) finalizeBurst(state *sprayState, steamID uint64, demoStats *DemoStats) {
	// Only process if we have enough bullets for analysis
	if state.bulletIndex < rc.minBurstSize || state.countedBullets == 0 {
		return
	}

	playerStats := demoStats.GetOrCreatePlayerStatsBySteamID(steamID)
	if playerStats == nil {
		return
	}

	// Track total error sum and bullet count for final calculation
	currentErrorSum := 0.0
	if metric, found := playerStats.GetMetric(Category("recoil"), Key("total_error_sum")); found {
		currentErrorSum = metric.FloatValue
	}

	playerStats.AddMetric(Category("recoil"), Key("total_error_sum"), Metric{
		Type:        MetricFloat,
		FloatValue:  currentErrorSum + state.sumError,
		Description: "Total angular error sum in degrees",
	})

	// Add bullet count
	for i := 0; i < state.countedBullets; i++ {
		playerStats.IncrementIntMetric(Category("recoil"), Key("total_counted_bullets"))
	}

	// Increment burst count
	playerStats.IncrementIntMetric(Category("recoil"), Key("burst_count"))

	// Also track weapon-specific metrics for deeper analysis
	weaponKey := Key(fmt.Sprintf("%s_bullets", weaponTypeToString(state.weapon)))
	currentWeaponCount := int64(0)
	if metric, found := playerStats.GetMetric(Category("recoil"), weaponKey); found {
		currentWeaponCount = metric.IntValue
	}

	playerStats.AddMetric(Category("recoil"), weaponKey, Metric{
		Type:        MetricInteger,
		IntValue:    currentWeaponCount + int64(state.countedBullets),
		Description: fmt.Sprintf("Bullets analyzed for %s", weaponTypeToString(state.weapon)),
	})

	// Reset the spray state
	state.inBurst = false
	state.bulletIndex = 0
	state.sumError = 0
	state.countedBullets = 0
}

// CollectFrame checks for expired bursts
func (rc *RecoilControlCollector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	currentTick := parser.CurrentFrame()

	// Check for expired bursts (player stopped firing)
	for steamID, state := range rc.sprayStates {
		if state.inBurst && (currentTick-state.lastFireTick > rc.maxBurstGap) {
			rc.finalizeBurst(state, steamID, demoStats)
		}
	}
}

// CollectFinalStats calculates final recoil control statistics
func (rc *RecoilControlCollector) CollectFinalStats(demoStats *DemoStats) {
	// Finalize any active bursts
	for steamID, state := range rc.sprayStates {
		if state.inBurst {
			rc.finalizeBurst(state, steamID, demoStats)
		}
	}

	// Calculate final stats for each player
	for _, playerStats := range demoStats.Players {
		totalErrorSum, foundError := playerStats.GetMetric(Category("recoil"), Key("total_error_sum"))
		totalBullets, foundBullets := playerStats.GetMetric(Category("recoil"), Key("total_counted_bullets"))
		burstCount, foundBursts := playerStats.GetMetric(Category("recoil"), Key("burst_count"))

		// Skip if insufficient data
		if !foundError || !foundBullets || !foundBursts ||
			totalBullets.IntValue < 10 || burstCount.IntValue < 2 {
			// Add N/A metric for players with insufficient data
			playerStats.AddMetric(Category("recoil"), Key("mean_angular_error"), Metric{
				Type:        MetricFloat,
				FloatValue:  -1, // -1 indicates insufficient data
				Description: "Mean angular error in recoil control (degrees)",
			})

			playerStats.AddMetric(Category("recoil"), Key("recoil_efficiency"), Metric{
				Type:        MetricPercentage,
				FloatValue:  0, // Default to 0% for insufficient data
				Description: "Recoil control efficiency (higher is more suspicious)",
			})

			continue
		}

		// Double check to avoid division by zero
		if totalBullets.IntValue <= 0 {
			continue
		}

		// Calculate mean angular error across all bursts
		meanError := totalErrorSum.FloatValue / float64(totalBullets.IntValue)

		// Store mean angular error
		playerStats.AddMetric(Category("recoil"), Key("mean_angular_error"), Metric{
			Type:        MetricFloat,
			FloatValue:  meanError,
			Description: "Mean angular error in recoil control (degrees)",
		})

		// Calculate recoil efficiency score using the corrected formula
		// 0% at 1.0 degrees or higher, 100% at 0.3 degrees or lower, linear in between
		recoilEfficiency := 0.0
		if meanError <= rc.perfectThreshold {
			recoilEfficiency = 100.0
		} else if meanError < 1.0 {
			// Linear scale from 0 to 100% between perfect (0.3°) and poor (1.0°)
			recoilEfficiency = 100.0 * (1.0 - ((meanError - rc.perfectThreshold) / (1.0 - rc.perfectThreshold)))
		}

		// Ensure we don't have negative efficiency from potential calculation issues
		if recoilEfficiency < 0.0 {
			recoilEfficiency = 0.0
		}

		playerStats.AddMetric(Category("recoil"), Key("recoil_efficiency"), Metric{
			Type:        MetricPercentage,
			FloatValue:  recoilEfficiency,
			Description: "Recoil control efficiency (higher is more suspicious)",
		})

		// Add interpretation
		playerStats.AddMetric(Category("recoil"), Key("recoil_interpretation"), Metric{
			Type:        MetricString,
			StringValue: interpretation(meanError, rc.perfectThreshold, rc.goodThreshold),
			Description: "Interpretation of recoil control ability",
		})
	}
}

// interpretation returns a string describing the recoil control quality based on mean error
func interpretation(meanError, perfectThreshold, goodThreshold float64) string {
	if meanError > 1.2 {
		return "Poor recoil control"
	} else if meanError <= perfectThreshold {
		return "Suspiciously perfect recoil control"
	} else if meanError <= goodThreshold {
		return "Very good recoil control"
	}
	return "Normal recoil control"
}

// weaponTypeToString converts weapon types to descriptive names
func weaponTypeToString(weaponType common.EquipmentType) string {
	switch weaponType {
	case common.EqAK47:
		return "ak47"
	case common.EqM4A4:
		return "m4a4"
	case common.EqM4A1:
		return "m4a1"
	case common.EqFamas:
		return "famas"
	case common.EqGalil:
		return "galil"
	case common.EqMP7:
		return "mp7"
	case common.EqMP9:
		return "mp9"
	case common.EqP90:
		return "p90"
	case common.EqUMP:
		return "ump"
	case common.EqNegev:
		return "negev"
	case common.EqM249:
		return "m249"
	case common.EqSG556:
		return "sg556"
	case common.EqAUG:
		return "aug"
	default:
		return "unknown"
	}
}

// isAutomaticWeapon returns true if the weapon type is automatic
func isAutomaticWeapon(weapon *common.Equipment) bool {
	if weapon == nil {
		return false
	}

	// Primary check - look for specific weapons by type
	switch weapon.Type {
	case common.EqAK47, common.EqM4A4, common.EqM4A1,
		common.EqFamas, common.EqGalil,
		common.EqMP7, common.EqMP9, common.EqP90,
		common.EqUMP, common.EqNegev,
		common.EqM249, common.EqSG556, common.EqAUG:
		return true
	}

	// Secondary check - include any rifle or SMG
	weaponClass := weapon.Class()
	if weaponClass == common.EqClassSMG || weaponClass == common.EqClassRifle {
		return true
	}

	return false
}

// getRecoilOffsets returns the expected yaw/pitch offsets for a specific weapon and bullet index
// These are approximations of the recoil patterns for different weapons
// Returns values in DEGREES
func getRecoilOffsets(weaponType common.EquipmentType, bulletIndex int) (float64, float64) {
	// Simplified recoil patterns (real game has more detailed patterns)
	// Values are in degrees

	// Clamp bullet index to prevent out-of-bounds access
	if bulletIndex < 1 {
		bulletIndex = 1
	} else if bulletIndex > 30 {
		bulletIndex = 30
	}

	// Approximate patterns for common automatic weapons
	// Format: {yaw offset, pitch offset} in DEGREES
	recoilPatterns := map[common.EquipmentType][][]float64{
		common.EqAK47: {
			{0.0, 0.0},   // Bullet 1 (no recoil)
			{0.0, 1.0},   // Bullet 2
			{0.0, 2.5},   // Bullet 3
			{0.2, 4.0},   // Bullet 4
			{0.5, 5.5},   // Bullet 5
			{1.0, 7.0},   // Bullet 6
			{2.0, 8.5},   // Bullet 7
			{3.0, 9.5},   // Bullet 8
			{3.5, 10.0},  // Bullet 9
			{2.5, 10.5},  // Bullet 10
			{0.0, 11.0},  // Bullet 11
			{-2.5, 11.5}, // Bullet 12
			{-4.0, 12.0}, // Bullet 13
			{-5.0, 12.5}, // Bullet 14
			{-5.5, 13.0}, // Bullet 15
			{-5.0, 13.5}, // Bullet 16
			{-4.0, 14.0}, // Bullet 17
			{-2.0, 14.5}, // Bullet 18
			{0.0, 15.0},  // Bullet 19
			{2.0, 15.5},  // Bullet 20
			{4.0, 16.0},  // Bullet 21
			{5.0, 16.5},  // Bullet 22
			{5.5, 17.0},  // Bullet 23
			{5.0, 17.5},  // Bullet 24
			{4.0, 18.0},  // Bullet 25
			{2.0, 18.5},  // Bullet 26
			{0.0, 19.0},  // Bullet 27
			{-2.0, 19.5}, // Bullet 28
			{-4.0, 20.0}, // Bullet 29
			{-5.0, 20.5}, // Bullet 30
		},
		common.EqM4A4: {
			{0.0, 0.0},   // Bullet 1
			{0.0, 0.8},   // Bullet 2
			{0.0, 2.0},   // Bullet 3
			{0.2, 3.5},   // Bullet 4
			{0.4, 5.0},   // Bullet 5
			{0.8, 6.2},   // Bullet 6
			{1.5, 7.0},   // Bullet 7
			{2.5, 7.5},   // Bullet 8
			{3.0, 8.0},   // Bullet 9
			{2.0, 8.5},   // Bullet 10
			{0.0, 9.0},   // Bullet 11
			{-2.0, 9.5},  // Bullet 12
			{-3.0, 10.0}, // Bullet 13
			{-3.5, 10.2}, // Bullet 14
			{-3.0, 10.5}, // Bullet 15
			{-1.5, 10.8}, // Bullet 16
			{0.0, 11.0},  // Bullet 17
			{1.5, 11.2},  // Bullet 18
			{2.5, 11.5},  // Bullet 19
			{3.0, 11.8},  // Bullet 20
		},
		common.EqM4A1: {
			{0.0, 0.0},  // Bullet 1
			{0.0, 0.7},  // Bullet 2
			{0.0, 1.8},  // Bullet 3
			{0.1, 3.0},  // Bullet 4
			{0.3, 4.5},  // Bullet 5
			{0.7, 5.5},  // Bullet 6
			{1.2, 6.2},  // Bullet 7
			{2.0, 6.8},  // Bullet 8
			{2.5, 7.2},  // Bullet 9
			{1.8, 7.6},  // Bullet 10
			{0.0, 8.0},  // Bullet 11
			{-1.8, 8.2}, // Bullet 12
			{-2.5, 8.5}, // Bullet 13
			{-3.0, 8.7}, // Bullet 14
			{-2.5, 9.0}, // Bullet 15
			{-1.0, 9.2}, // Bullet 16
			{0.0, 9.5},  // Bullet 17
			{1.0, 9.7},  // Bullet 18
			{2.0, 10.0}, // Bullet 19
			{2.5, 10.2}, // Bullet 20
		},
	}

	// Get pattern for this weapon
	pattern, exists := recoilPatterns[weaponType]
	if !exists || len(pattern) == 0 {
		// Default pattern if specific weapon not defined
		// Approximation: mostly vertical recoil increasing with bullet count
		yawOffset := 0.0
		if bulletIndex > 10 {
			// After bullet 10, add some horizontal movement
			phase := float64(bulletIndex-10) * 0.6
			yawOffset = math.Sin(phase) * float64(bulletIndex) * 0.3
		}
		pitchOffset := math.Min(float64(bulletIndex)*0.7, 20.0)
		return yawOffset, pitchOffset
	}

	// If we have fewer pattern entries than the bullet index
	if bulletIndex-1 >= len(pattern) {
		return pattern[len(pattern)-1][0], pattern[len(pattern)-1][1]
	}

	return pattern[bulletIndex-1][0], pattern[bulletIndex-1][1]
}
