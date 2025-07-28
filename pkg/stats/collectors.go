package stats

import (
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
)

// Collector is the interface for all statistics collectors
type Collector interface {
	// Name returns the name of this collector
	Name() string

	// Categories returns the categories of statistics this collector generates
	Categories() []Category

	// Setup is called once before parsing starts to set up event handlers, etc.
	Setup(parser demoinfocs.Parser, demoStats *DemoStats)

	// CollectFrame is called for each parsed frame
	CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats)

	// CollectFinalStats is called after parsing is complete to calculate final stats
	CollectFinalStats(demoStats *DemoStats)
}

// BaseCollector provides common functionality for statistics collectors
type BaseCollector struct {
	name       string
	categories []Category
}

// NewBaseCollector creates a new BaseCollector
func NewBaseCollector(name string, categories ...Category) *BaseCollector {
	return &BaseCollector{
		name:       name,
		categories: categories,
	}
}

// Name returns the name of this collector
func (bc *BaseCollector) Name() string {
	return bc.name
}

// Categories returns the categories of statistics this collector generates
func (bc *BaseCollector) Categories() []Category {
	return bc.categories
}

// Setup is called once before parsing starts
func (bc *BaseCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	// Empty base implementation
}

// CollectFrame is called for each parsed frame
func (bc *BaseCollector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	// Empty base implementation
}

// CollectFinalStats is called after parsing is complete
func (bc *BaseCollector) CollectFinalStats(demoStats *DemoStats) {
	// Empty base implementation
}

// WeaponUsageCollector tracks weapon usage statistics
type WeaponUsageCollector struct {
	*BaseCollector
}

// NewWeaponUsageCollector creates a new WeaponUsageCollector
func NewWeaponUsageCollector() *WeaponUsageCollector {
	return &WeaponUsageCollector{
		BaseCollector: NewBaseCollector("Weapon Usage", Category("weapons")),
	}
}

// CollectFrame implements weapon usage collection per frame
func (wuc *WeaponUsageCollector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	gs := parser.GameState()

	for _, player := range gs.Participants().Playing() {
		if player == nil || player.SteamID64 == 0 {
			continue
		}

		playerStats := demoStats.GetOrCreatePlayerStats(player)
		if playerStats == nil {
			continue
		}

		// Track total ticks for this player
		playerStats.IncrementIntMetric(Category("weapons"), Key("total_ticks"))

		// Get active weapon
		activeWeapon := player.ActiveWeapon()
		if activeWeapon == nil {
			// Track no-weapon ticks
			playerStats.IncrementIntMetric(Category("weapons"), Key("no_weapon_ticks"))
			continue
		}

		// Track weapon-specific ticks
		if isKnife(activeWeapon) {
			playerStats.IncrementIntMetric(Category("weapons"), Key("knife_ticks"))
		} else {
			playerStats.IncrementIntMetric(Category("weapons"), Key("non_knife_ticks"))
		}
	}
}

// CollectFinalStats calculates percentage statistics after parsing
func (wuc *WeaponUsageCollector) CollectFinalStats(demoStats *DemoStats) {
	for _, playerStats := range demoStats.Players {
		totalTicks, found := playerStats.GetMetric(Category("weapons"), Key("total_ticks"))
		if !found || totalTicks.IntValue == 0 {
			continue
		}

		// Calculate knife percentage
		if knifeTicks, found := playerStats.GetMetric(Category("weapons"), Key("knife_ticks")); found {
			knifePercentage := float64(knifeTicks.IntValue) / float64(totalTicks.IntValue) * 100
			playerStats.AddMetric(Category("weapons"), Key("knife_percentage"), Metric{
				Type:        MetricPercentage,
				FloatValue:  knifePercentage,
				Description: "Percentage of time with knife equipped",
			})
		}

		// Calculate non-knife percentage
		if nonKnifeTicks, found := playerStats.GetMetric(Category("weapons"), Key("non_knife_ticks")); found {
			nonKnifePercentage := float64(nonKnifeTicks.IntValue) / float64(totalTicks.IntValue) * 100
			playerStats.AddMetric(Category("weapons"), Key("non_knife_percentage"), Metric{
				Type:        MetricPercentage,
				FloatValue:  nonKnifePercentage,
				Description: "Percentage of time with non-knife weapons equipped",
			})
		}

		// Calculate no-weapon percentage
		if noWeaponTicks, found := playerStats.GetMetric(Category("weapons"), Key("no_weapon_ticks")); found {
			noWeaponPercentage := float64(noWeaponTicks.IntValue) / float64(totalTicks.IntValue) * 100
			playerStats.AddMetric(Category("weapons"), Key("no_weapon_percentage"), Metric{
				Type:        MetricPercentage,
				FloatValue:  noWeaponPercentage,
				Description: "Percentage of time with no weapon equipped",
			})
		}
		
		// Validate percentages add up to 100%
		knifePerc := 0.0
		nonKnifePerc := 0.0
		noWeaponPerc := 0.0
		
		if metric, found := playerStats.GetMetric(Category("weapons"), Key("knife_percentage")); found {
			knifePerc = metric.FloatValue
		}
		if metric, found := playerStats.GetMetric(Category("weapons"), Key("non_knife_percentage")); found {
			nonKnifePerc = metric.FloatValue
		}
		if metric, found := playerStats.GetMetric(Category("weapons"), Key("no_weapon_percentage")); found {
			noWeaponPerc = metric.FloatValue
		}
		
		totalPerc := knifePerc + nonKnifePerc + noWeaponPerc
		if totalPerc < 99.9 || totalPerc > 100.1 {
			// There might be rounding issues, but we should be close to 100%
			// Log an error or handle the case where the percentages don't add up
		}
	}
}

// isKnife checks if an equipment is a knife
func isKnife(weapon *common.Equipment) bool {
	if weapon == nil {
		return false
	}

	// Check if the weapon is knife by type or name
	weaponName := weapon.String()

	return weapon.Type == common.EqKnife ||
		weaponName == "Knife" ||
		weaponName == "Bayonet" ||
		weaponName == "Karambit"
}
