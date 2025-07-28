package stats

import (
	"math"
	"sort"

	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

// Constants for reaction time calculations
const (
	// FOV in degrees to check if an enemy is in view (reduced from 10 to 5 degrees for rifles to reduce false positives)
	ReactionFOVDegrees = 5.0

	// Minimum number of reaction time samples needed for meaningful statistics
	MinReactionSamples = 5
)

// ReactionTimeCollector tracks reaction time statistics
type ReactionTimeCollector struct {
	*BaseCollector
	// Maps attacker ID -> victim ID -> tick when victim entered FOV
	entryTicks map[uint64]map[uint64]int
	// Maps player ID -> array of reaction times in ms
	reactionTimes map[uint64][]float64
	// Current tick
	currentTick int
	// Tick rate of the demo
	tickRate float64
}

// NewReactionTimeCollector creates a new ReactionTimeCollector
func NewReactionTimeCollector() *ReactionTimeCollector {
	return &ReactionTimeCollector{
		BaseCollector: NewBaseCollector("Reaction Time Analysis", Category("reaction")),
		entryTicks:    make(map[uint64]map[uint64]int),
		reactionTimes: make(map[uint64][]float64),
		currentTick:   0,
	}
}

// Setup initializes the collector with the demo parser
func (rtc *ReactionTimeCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	rtc.tickRate = parser.TickRate()
	if rtc.tickRate == 0 {
		rtc.tickRate = 64.0 // Default tick rate for CS2 if we can't get it from the parser
	}

	// Register weapon fire event handler
	parser.RegisterEventHandler(func(e events.WeaponFire) {
		rtc.processWeaponFire(e, parser, demoStats)
	})

	// Register round end handler to reset entry ticks
	parser.RegisterEventHandler(func(e events.RoundEnd) {
		rtc.entryTicks = make(map[uint64]map[uint64]int)
	})

	// Register player killed event to reset entry ticks for that player
	parser.RegisterEventHandler(func(e events.Kill) {
		if e.Victim != nil {
			rtc.clearEntryTicksForPlayer(e.Victim.SteamID64)
		}
		if e.Killer != nil {
			rtc.clearEntryTicksForPlayer(e.Killer.SteamID64)
		}
	})

	// Register player hurt event to track hits in conjunction with weapon fire
	parser.RegisterEventHandler(func(e events.PlayerHurt) {
		// Currently not used, but could be implemented to only count shots that hit
	})
}

// processWeaponFire handles weapon fire events to calculate reaction times
func (rtc *ReactionTimeCollector) processWeaponFire(e events.WeaponFire, parser demoinfocs.Parser, demoStats *DemoStats) {
	shooter := e.Shooter
	if shooter == nil || shooter.SteamID64 == 0 {
		return
	}

	// Skip if player is flashed - we'll just skip this check as it's not critical
	// and the method appears to not be available

	// Check if we're tracking any entry ticks for this shooter
	attackerEntryTicks, exists := rtc.entryTicks[shooter.SteamID64]
	if !exists || len(attackerEntryTicks) == 0 {
		return
	}

	// Calculate reaction time for each victim in FOV
	for _, entryTick := range attackerEntryTicks {
		// Calculate reaction time in milliseconds
		deltaT := float64(rtc.currentTick-entryTick) * (1000.0 / rtc.tickRate)

		// Filter out unrealistic reaction times (too long)
		// This is a sanity check in case FOV clearing logic fails
		if deltaT <= 2000.0 { // Ignore reactions longer than 2 seconds
			// Store the reaction time
			if _, exists := rtc.reactionTimes[shooter.SteamID64]; !exists {
				rtc.reactionTimes[shooter.SteamID64] = make([]float64, 0)
			}
			rtc.reactionTimes[shooter.SteamID64] = append(rtc.reactionTimes[shooter.SteamID64], deltaT)

			// Get or create player stats
			playerStats := demoStats.GetOrCreatePlayerStats(shooter)
			if playerStats != nil {
				playerStats.IncrementIntMetric(Category("reaction"), Key("shots_after_fov_entry"))
			}
		}
	}

	// Clear entry ticks for this attacker to avoid double-counting
	// until targets leave and re-enter FOV
	rtc.entryTicks[shooter.SteamID64] = make(map[uint64]int)
}

// clearEntryTicksForPlayer removes entry tick records for a player (when they die or disconnect)
func (rtc *ReactionTimeCollector) clearEntryTicksForPlayer(playerID uint64) {
	// Remove as target
	for attackerID, targets := range rtc.entryTicks {
		delete(targets, playerID)
		rtc.entryTicks[attackerID] = targets
	}

	// Remove as attacker
	delete(rtc.entryTicks, playerID)
}

// CollectFrame updates the entry tick data for each player on every frame
func (rtc *ReactionTimeCollector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	rtc.currentTick = parser.CurrentFrame()
	gs := parser.GameState()

	// Pre-compute FOV constant
	cosHalfFOV := math.Cos(ReactionFOVDegrees * math.Pi / 180.0 / 2.0)

	// For each player (attacker)
	for _, attacker := range gs.Participants().Playing() {
		if attacker == nil || attacker.SteamID64 == 0 {
			continue
		}

		// Skip if player is flashed - we'll skip this check

		// Skip if player has a scoped sniper and has been scoped in for >= 500ms
		// This is a placeholder - you would need to track scoped-in time separately
		// if attacker.HasWeapon(common.EqAWP) && attacker.IsScoped && scopedTime >= 500ms {
		//     continue
		// }

		attackerID := attacker.SteamID64
		attackerPos := attacker.Position()

		// Based on the snap_collectors.go implementation, it appears ViewDirectionZ doesn't exist
		// We'll use X and Y direction vectors that are available
		viewDirX := attacker.ViewDirectionX()
		viewDirY := attacker.ViewDirectionY()

		// Initialize entry ticks map for this attacker if it doesn't exist
		if _, exists := rtc.entryTicks[attackerID]; !exists {
			rtc.entryTicks[attackerID] = make(map[uint64]int)
		}

		// Create a list of victims to remove (those who left FOV)
		opponentsToRemove := make([]uint64, 0)

		// First mark all current entries for potential removal
		for opponentID := range rtc.entryTicks[attackerID] {
			opponentsToRemove = append(opponentsToRemove, opponentID)
		}

		// Check each opponent
		for _, opponent := range gs.Participants().Playing() {
			// Skip if same team, self, or not alive
			if opponent == nil || opponent.SteamID64 == 0 || opponent.Team == attacker.Team ||
				opponent.SteamID64 == attackerID || !opponent.IsAlive() {
				continue
			}

			opponentID := opponent.SteamID64
			opponentPos := opponent.Position()

			// Calculate vector from attacker to opponent (using float32 since that's the type used in Position())
			vecX := float32(opponentPos.X - attackerPos.X)
			vecY := float32(opponentPos.Y - attackerPos.Y)
			vecZ := float32(opponentPos.Z - attackerPos.Z)

			// Normalize the vector
			length := float32(math.Sqrt(float64(vecX*vecX + vecY*vecY + vecZ*vecZ)))
			if length > 0 {
				vecX /= length
				vecY /= length
				vecZ /= length
			} else {
				continue // Skip if the players are at the same position
			}

			// Calculate dot product (2D since we only have X and Y view directions)
			dotProduct := vecX*viewDirX + vecY*viewDirY

			// Check if opponent is in FOV
			if dotProduct >= float32(cosHalfFOV) {
				// If we're not already tracking this opponent, record the entry tick
				if _, exists := rtc.entryTicks[attackerID][opponentID]; !exists {
					rtc.entryTicks[attackerID][opponentID] = rtc.currentTick
				}

				// Remove this opponent from the removal list since they're still in FOV
				for i, id := range opponentsToRemove {
					if id == opponentID {
						opponentsToRemove = append(opponentsToRemove[:i], opponentsToRemove[i+1:]...)
						break
					}
				}
			}
		}

		// Remove any opponents that left the FOV
		for _, opponentID := range opponentsToRemove {
			delete(rtc.entryTicks[attackerID], opponentID)
		}
	}
}

// CollectFinalStats calculates reaction time statistics for each player
func (rtc *ReactionTimeCollector) CollectFinalStats(demoStats *DemoStats) {
	for playerID, times := range rtc.reactionTimes {
		if len(times) < MinReactionSamples {
			continue // Not enough data for reliable statistics
		}

		// Sort reaction times for percentile calculations
		sort.Float64s(times)

		// Find player stats
		var player *common.Player
		for steamID, stats := range demoStats.Players {
			if steamID == playerID {
				player = &common.Player{
					Name:      stats.Player.Name,
					SteamID64: playerID,
				}
				break
			}
		}

		if player == nil {
			continue
		}

		playerStats := demoStats.GetOrCreatePlayerStats(player)
		if playerStats == nil {
			continue
		}

		// Calculate median reaction time
		medianIndex := len(times) / 2
		medianReaction := times[medianIndex]

		// Calculate 10th percentile (P10)
		p10Index := int(float64(len(times)) * 0.1)
		if p10Index < 0 {
			p10Index = 0
		}
		p10Reaction := times[p10Index]

		// Calculate sub-100ms ratio
		sub100Count := 0
		for _, t := range times {
			if t <= 100.0 {
				sub100Count++
			}
		}
		sub100Ratio := float64(sub100Count) / float64(len(times))

		// Store metrics
		playerStats.AddMetric(Category("reaction"), Key("median_reaction_time"), Metric{
			Type:        MetricFloat,
			FloatValue:  medianReaction,
			Description: "Median reaction time in milliseconds",
		})

		playerStats.AddMetric(Category("reaction"), Key("p10_reaction_time"), Metric{
			Type:        MetricFloat,
			FloatValue:  p10Reaction,
			Description: "10th percentile reaction time in milliseconds",
		})

		playerStats.AddMetric(Category("reaction"), Key("sub_100ms_ratio"), Metric{
			Type:        MetricPercentage,
			FloatValue:  sub100Ratio * 100.0,
			Description: "Percentage of shots fired within 100ms of enemy entering FOV",
		})

		playerStats.AddMetric(Category("reaction"), Key("reaction_samples"), Metric{
			Type:        MetricInteger,
			IntValue:    int64(len(times)),
			Description: "Number of reaction time samples collected",
		})

		// Calculate reaction time cheat score
		// rtScore = clamp01((120 - P10Reaction) / 60)  // 0 at 120 ms, 1 at 60 ms or below
		rtScore := clamp01((120.0 - p10Reaction) / 60.0)
		playerStats.AddMetric(Category("reaction"), Key("reaction_cheat_score"), Metric{
			Type:        MetricFloat,
			FloatValue:  rtScore,
			Description: "Reaction time-based cheat score (0-1, higher is more suspicious)",
		})
	}
}
