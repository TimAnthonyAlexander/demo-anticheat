package analyzer

import (
	"fmt"
	"os"
	"time"

	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
)

// Analyzer represents a CS2 demo analyzer
type Analyzer struct {
	demoPath string
}

// PlayerWeaponStats contains statistics about a player's weapon usage
type PlayerWeaponStats struct {
	PlayerName      string
	SteamID64       uint64
	KnifeTicks      int64
	NonKnifeTicks   int64
	TotalTicks      int64
	KnifePercent    float64
	NonKnifePercent float64
}

// Results represents the analysis results
type Results struct {
	PlayerStats map[uint64]*PlayerWeaponStats
	TickRate    float64
}

// NewAnalyzer creates a new analyzer for the given demo file
func NewAnalyzer(demoPath string) *Analyzer {
	return &Analyzer{
		demoPath: demoPath,
	}
}

// Analyze performs the analysis of the demo file
func (a *Analyzer) Analyze() (Results, error) {
	// Open the demo file
	f, err := os.Open(a.demoPath)
	if err != nil {
		return Results{}, fmt.Errorf("failed to open demo file: %w", err)
	}
	defer f.Close()

	// Create a new parser
	parser := dem.NewParser(f)
	defer parser.Close()

	// Parse the header
	_, err = parser.ParseHeader()
	if err != nil {
		return Results{}, fmt.Errorf("failed to parse demo header: %w", err)
	}

	// Get the tick rate
	tickRate := parser.TickRate()
	stats := make(map[uint64]*PlayerWeaponStats)

	// Parse all frames
	for {
		// Parse the next frame
		ok, err := parser.ParseNextFrame()
		if err != nil {
			return Results{}, fmt.Errorf("error parsing frame: %w", err)
		}

		// Check if we've reached the end of the demo
		if !ok {
			break
		}

		// Get the game state
		gs := parser.GameState()

		// Analyze all playing participants
		for _, p := range gs.Participants().Playing() {
			if p == nil || p.SteamID64 == 0 {
				continue
			}

			// Get player's active weapon
			activeWeapon := p.ActiveWeapon()
			if activeWeapon == nil {
				continue
			}

			// Initialize player stats if not exists
			if _, ok := stats[p.SteamID64]; !ok {
				stats[p.SteamID64] = &PlayerWeaponStats{
					PlayerName: p.Name,
					SteamID64:  p.SteamID64,
				}
			}

			// Update the player's stats
			stats[p.SteamID64].TotalTicks++

			// Check if weapon is knife by name (most reliable approach)
			if isKnife(activeWeapon) {
				stats[p.SteamID64].KnifeTicks++
			} else {
				stats[p.SteamID64].NonKnifeTicks++
			}
		}
	}

	// Calculate percentages
	for _, playerStats := range stats {
		if playerStats.TotalTicks > 0 {
			playerStats.KnifePercent = float64(playerStats.KnifeTicks) / float64(playerStats.TotalTicks) * 100
			playerStats.NonKnifePercent = float64(playerStats.NonKnifeTicks) / float64(playerStats.TotalTicks) * 100
		}
	}

	return Results{
		PlayerStats: stats,
		TickRate:    tickRate,
	}, nil
}

// isKnife checks if an equipment is a knife
func isKnife(weapon *common.Equipment) bool {
	if weapon == nil {
		return false
	}

	// Check if the weapon name contains "knife" or is specific knife type
	weaponName := weapon.String()

	return weapon.Type == common.EqKnife ||
		weaponName == "Knife" ||
		weaponName == "Bayonet" ||
		weaponName == "Karambit"
}

// GetTimeFromTicks converts tick count to time.Duration
func (a *Analyzer) GetTimeFromTicks(ticks int64, tickRate float64) time.Duration {
	seconds := float64(ticks) / tickRate
	return time.Duration(seconds * float64(time.Second))
}
