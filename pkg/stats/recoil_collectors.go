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
	RecoilRadToDeg = 57.295779513
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
	burstIDCounter   int  // For debug output
}

// sprayState tracks the state of a player's weapon spray
type sprayState struct {
	inBurst        bool
	burstID        int
	firstTick      int
	firstYawDeg    float64 // In degrees
	firstPitchDeg  float64 // In degrees
	bulletIndex    int
	lastFireTick   int
	weapon         common.EquipmentType
	weaponName     string
	sumError       float64
	countedBullets int
}

// NewRecoilControlCollector creates a new RecoilControlCollector
func NewRecoilControlCollector() *RecoilControlCollector {
	return &RecoilControlCollector{
		BaseCollector:    NewBaseCollector("Recoil Control", Category("recoil")),
		sprayStates:      make(map[uint64]*sprayState),
		maxBurstGap:      6,     // Ticks between shots to consider it part of the same burst
		minBurstSize:     4,     // Minimum bullets to consider a valid burst
		maxBulletIdx:     30,    // Maximum bullets to track in a spray pattern
		goodThreshold:    0.7,   // Threshold for good recoil control (in degrees)
		perfectThreshold: 0.3,   // Threshold for suspiciously perfect recoil control (in degrees)
		debugMode:        false, // Set to false in production
		burstIDCounter:   1,     // Start at 1
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

	// Get weapon name for debugging
	weaponName := getWeaponName(weapon)

	// Get view angles in DEGREES
	actualYawRad := float64(shooter.ViewDirectionX())
	actualPitchRad := float64(shooter.ViewDirectionY())
	actualYawDeg := actualYawRad * RecoilRadToDeg
	actualPitchDeg := actualPitchRad * RecoilRadToDeg

	steamID := shooter.SteamID64
	state, exists := rc.sprayStates[steamID]

	// If player has no spray state or we need to start a new burst
	if !exists {
		burstID := rc.burstIDCounter
		rc.burstIDCounter++

		rc.sprayStates[steamID] = &sprayState{
			inBurst:       true,
			burstID:       burstID,
			firstTick:     currentTick,
			firstYawDeg:   actualYawDeg,
			firstPitchDeg: actualPitchDeg,
			bulletIndex:   1,
			lastFireTick:  currentTick,
			weapon:        weapon.Type,
			weaponName:    weaponName,
		}

		// Log first bullet info for debugging
		if rc.debugMode {
			fmt.Printf("[DEBUG] B%02d Player:%d Weapon:%s First bullet angles: Yaw=%.2f° Pitch=%.2f°\n",
				burstID, steamID, weaponName, actualYawDeg, actualPitchDeg)
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
				expectedYawOffset, expectedPitchOffset, ok := getRecoilOffsets(state.weapon, state.bulletIndex)

				// Skip this bullet if pattern lookup failed
				if !ok {
					if rc.debugMode {
						fmt.Printf("[DEBUG] B%02d Player:%d Weapon:%s Bullet:%d - No spray pattern found\n",
							state.burstID, steamID, state.weaponName, state.bulletIndex)
					}
					state.lastFireTick = currentTick
					return
				}

				// Calculate expected aim angles (in degrees)
				// We subtract offsets because we want to compensate for recoil
				expectedYawDeg := state.firstYawDeg - expectedYawOffset
				expectedPitchDeg := state.firstPitchDeg - expectedPitchOffset

				// Calculate angular error (in degrees)
				yawDiffDeg := expectedYawDeg - actualYawDeg
				pitchDiffDeg := expectedPitchDeg - actualPitchDeg
				angularErrorDeg := math.Hypot(yawDiffDeg, pitchDiffDeg)

				// Add to player's accumulated error (in degrees)
				state.sumError += angularErrorDeg
				state.countedBullets++

				// Debug output for every bullet
				if rc.debugMode {
					fmt.Printf("[DEBUG] B%02d Player:%d %s Bullet:%d Error:%.2f° Sum:%.2f Count:%d\n",
						state.burstID, steamID, state.weaponName, state.bulletIndex,
						angularErrorDeg, state.sumError, state.countedBullets)
				}
			}

			// Update last fire tick
			state.lastFireTick = currentTick
		} else {
			// Gap too large, end previous burst and start a new one
			rc.finalizeBurst(state, steamID, demoStats)

			burstID := rc.burstIDCounter
			rc.burstIDCounter++

			rc.sprayStates[steamID] = &sprayState{
				inBurst:       true,
				burstID:       burstID,
				firstTick:     currentTick,
				firstYawDeg:   actualYawDeg,
				firstPitchDeg: actualPitchDeg,
				bulletIndex:   1,
				lastFireTick:  currentTick,
				weapon:        weapon.Type,
				weaponName:    weaponName,
			}
		}
	} else {
		// Start a new burst if not in one
		burstID := rc.burstIDCounter
		rc.burstIDCounter++

		rc.sprayStates[steamID] = &sprayState{
			inBurst:       true,
			burstID:       burstID,
			firstTick:     currentTick,
			firstYawDeg:   actualYawDeg,
			firstPitchDeg: actualPitchDeg,
			bulletIndex:   1,
			lastFireTick:  currentTick,
			weapon:        weapon.Type,
			weaponName:    weaponName,
		}
	}
}

// finalizeBurst processes the end of a burst and calculates statistics
func (rc *RecoilControlCollector) finalizeBurst(state *sprayState, steamID uint64, demoStats *DemoStats) {
	// Only process if we have enough bullets for analysis
	if state.bulletIndex < rc.minBurstSize || state.countedBullets == 0 {
		if rc.debugMode {
			fmt.Printf("[DEBUG] B%02d Player:%d %s - Skipped burst: bullets=%d, counted=%d\n",
				state.burstID, steamID, state.weaponName, state.bulletIndex, state.countedBullets)
		}
		return
	}

	playerStats := demoStats.GetOrCreatePlayerStatsBySteamID(steamID)
	if playerStats == nil {
		return
	}

	// Calculate mean error for this burst
	meanError := state.sumError / float64(state.countedBullets)

	if rc.debugMode {
		fmt.Printf("[DEBUG] B%02d Player:%d %s - Burst finalized: bullets=%d, sum=%.2f°, mean=%.2f°\n",
			state.burstID, steamID, state.weaponName, state.countedBullets, state.sumError, meanError)
	}

	// Track total error sum and bullet count for final calculation
	currentErrorSum := 0.0
	currentBulletCount := int64(0)

	if metric, found := playerStats.GetMetric(Category("recoil"), Key("total_error_sum")); found {
		currentErrorSum = metric.FloatValue
	}

	if metric, found := playerStats.GetMetric(Category("recoil"), Key("total_counted_bullets")); found {
		currentBulletCount = metric.IntValue
	}

	// Update total error sum
	playerStats.AddMetric(Category("recoil"), Key("total_error_sum"), Metric{
		Type:        MetricFloat,
		FloatValue:  currentErrorSum + state.sumError,
		Description: "Total angular error sum in degrees",
	})

	// Update total bullet count
	playerStats.AddMetric(Category("recoil"), Key("total_counted_bullets"), Metric{
		Type:        MetricInteger,
		IntValue:    currentBulletCount + int64(state.countedBullets),
		Description: "Total bullets analyzed for recoil control",
	})

	// Increment burst count
	playerStats.IncrementIntMetric(Category("recoil"), Key("burst_count"))

	// Also track weapon-specific metrics
	weaponKey := Key(fmt.Sprintf("%s_bullets", weaponTypeToString(state.weapon)))
	currentWeaponCount := int64(0)
	if metric, found := playerStats.GetMetric(Category("recoil"), weaponKey); found {
		currentWeaponCount = metric.IntValue
	}

	playerStats.AddMetric(Category("recoil"), weaponKey, Metric{
		Type:        MetricInteger,
		IntValue:    currentWeaponCount + int64(state.countedBullets),
		Description: fmt.Sprintf("Bullets analyzed for %s", state.weaponName),
	})

	// Add burst-specific mean error for debugging
	if rc.debugMode {
		burstKey := Key(fmt.Sprintf("burst_%d_mean_error", state.burstID))
		playerStats.AddMetric(Category("recoil_debug"), burstKey, Metric{
			Type:        MetricFloat,
			FloatValue:  meanError,
			Description: fmt.Sprintf("Mean error for burst #%d with %s", state.burstID, state.weaponName),
		})
	}

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
	for steamID, playerStats := range demoStats.Players {
		totalErrorSum, foundError := playerStats.GetMetric(Category("recoil"), Key("total_error_sum"))
		totalBullets, foundBullets := playerStats.GetMetric(Category("recoil"), Key("total_counted_bullets"))
		burstCount, foundBursts := playerStats.GetMetric(Category("recoil"), Key("burst_count"))

		// Skip if insufficient data
		if !foundError || !foundBullets || !foundBursts ||
			totalBullets.IntValue < 10 || burstCount.IntValue < 2 {

			if rc.debugMode {
				fmt.Printf("[DEBUG] Player:%d - Insufficient data: errorFound=%v, bulletsFound=%v, burstsFound=%v, bullets=%d, bursts=%d\n",
					steamID, foundError, foundBullets, foundBursts,
					totalBullets.IntValue, burstCount.IntValue)
			}

			playerStats.AddMetric(Category("recoil"), Key("mean_angular_error"), Metric{
				Type:        MetricFloat,
				FloatValue:  0,
				Description: "Mean angular error in recoil control (degrees) - insufficient data",
			})

			playerStats.AddMetric(Category("recoil"), Key("recoil_efficiency"), Metric{
				Type:        MetricPercentage,
				FloatValue:  0,
				Description: "Recoil control efficiency (higher is more suspicious) - insufficient data",
			})

			continue
		}

		// Double check to avoid division by zero
		if totalBullets.IntValue <= 0 {
			continue
		}

		// Calculate mean angular error across all bursts
		meanError := totalErrorSum.FloatValue / float64(totalBullets.IntValue)

		if rc.debugMode {
			fmt.Printf("[DEBUG] Player:%d - Final calculation: bullets=%d, sum=%.2f°, mean=%.2f°\n",
				steamID, totalBullets.IntValue, totalErrorSum.FloatValue, meanError)
		}

		// Store mean angular error
		playerStats.AddMetric(Category("recoil"), Key("mean_angular_error"), Metric{
			Type:        MetricFloat,
			FloatValue:  meanError,
			Description: "Mean angular error in recoil control (degrees)",
		})

		// Calculate recoil efficiency score
		// 0% at 1.0 degrees or higher, 100% at 0.3 degrees or lower, linear in between
		recoilEfficiency := 0.0
		if meanError <= rc.perfectThreshold {
			recoilEfficiency = 100.0
		} else if meanError < 1.0 {
			// Linear scale from perfect (0.3°) to poor (1.0°)
			recoilEfficiency = 100.0 * (1.0 - ((meanError - rc.perfectThreshold) / (1.0 - rc.perfectThreshold)))
		}

		// Ensure efficiency is between 0-100%
		recoilEfficiency = math.Max(0.0, math.Min(100.0, recoilEfficiency))

		playerStats.AddMetric(Category("recoil"), Key("recoil_efficiency"), Metric{
			Type:        MetricPercentage,
			FloatValue:  recoilEfficiency,
			Description: "Recoil control efficiency (higher is more suspicious)",
		})

		// Calculate recoil score for the cheat detector using the specified formula
		// 0 at 0.75° or higher, 1 at 0.30° or lower, linear in between
		recoilScore := clamp01((0.75 - meanError) / 0.45)

		playerStats.AddMetric(Category("recoil"), Key("recoil_score"), Metric{
			Type:        MetricFloat,
			FloatValue:  recoilScore,
			Description: "Recoil score component for cheat detection (0-1)",
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

// getWeaponName returns a readable name for the weapon
func getWeaponName(weapon *common.Equipment) string {
	if weapon == nil {
		return "Unknown"
	}

	// Get display name from CS2 metadata if available
	name := weapon.String()
	if name != "" && name != "Unknown" {
		return name
	}

	// Fallback to our own mapping
	return weaponTypeToString(weapon.Type)
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

	// Check by weapon name first (most reliable in CS2)
	name := weapon.String()
	switch name {
	case "AK-47", "M4A1", "M4A4", "M4A1-S", "FAMAS", "Galil AR",
		"SG 553", "SG 556", "AUG", "MP9", "MAC-10", "MP7", "P90",
		"UMP-45", "PP-Bizon", "Negev", "M249":
		return true
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

	// Secondary check - include any rifle or SMG with multiple bullets
	weaponClass := weapon.Class()
	if weaponClass == common.EqClassSMG || weaponClass == common.EqClassRifle {
		return true
	}

	return false
}

// getRecoilOffsets returns the expected yaw/pitch offsets for a specific weapon and bullet index
// These are approximations of the recoil patterns for different weapons
// Returns values in DEGREES and a boolean indicating if the lookup succeeded
func getRecoilOffsets(weaponType common.EquipmentType, bulletIndex int) (float64, float64, bool) {
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
		// Add patterns for other common weapons
		common.EqMP9: {
			{0.0, 0.0},  // Bullet 1
			{0.0, 0.6},  // Bullet 2
			{0.0, 1.5},  // Bullet 3
			{0.2, 2.2},  // Bullet 4
			{0.5, 3.0},  // Bullet 5
			{1.0, 3.8},  // Bullet 6
			{1.5, 4.5},  // Bullet 7
			{2.0, 5.0},  // Bullet 8
			{1.5, 5.5},  // Bullet 9
			{0.5, 6.0},  // Bullet 10
			{-0.5, 6.3}, // Bullet 11
			{-1.5, 6.6}, // Bullet 12
			{-2.0, 6.9}, // Bullet 13
			{-1.5, 7.2}, // Bullet 14
			{-0.5, 7.5}, // Bullet 15
			{0.5, 7.8},  // Bullet 16
			{1.5, 8.1},  // Bullet 17
			{2.0, 8.4},  // Bullet 18
			{1.5, 8.7},  // Bullet 19
			{0.5, 9.0},  // Bullet 20
		},
		common.EqP90: {
			{0.0, 0.0},  // Bullet 1
			{0.0, 0.4},  // Bullet 2
			{0.0, 1.0},  // Bullet 3
			{0.1, 1.8},  // Bullet 4
			{0.2, 2.5},  // Bullet 5
			{0.4, 3.2},  // Bullet 6
			{0.7, 3.8},  // Bullet 7
			{1.0, 4.2},  // Bullet 8
			{1.3, 4.5},  // Bullet 9
			{1.0, 4.8},  // Bullet 10
			{0.5, 5.1},  // Bullet 11
			{0.0, 5.3},  // Bullet 12
			{-0.5, 5.5}, // Bullet 13
			{-1.0, 5.7}, // Bullet 14
			{-1.3, 5.9}, // Bullet 15
			{-1.0, 6.1}, // Bullet 16
			{-0.5, 6.3}, // Bullet 17
			{0.0, 6.5},  // Bullet 18
			{0.5, 6.7},  // Bullet 19
			{1.0, 6.9},  // Bullet 20
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
		return yawOffset, pitchOffset, true
	}

	// If we have fewer pattern entries than the bullet index
	if bulletIndex-1 >= len(pattern) {
		if len(pattern) == 0 {
			return 0, 0, false // Empty pattern, bail out
		}
		return pattern[len(pattern)-1][0], pattern[len(pattern)-1][1], true
	}

	return pattern[bulletIndex-1][0], pattern[bulletIndex-1][1], true
}
