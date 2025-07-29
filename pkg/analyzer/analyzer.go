package analyzer

import (
	"fmt"
	"os"
	"path/filepath"

	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	"github.com/timanthonyalexander/demo-anticheat/pkg/stats"
)

// Analyzer represents a CS2 demo analyzer
type Analyzer struct {
	demoPath   string
	collectors []stats.Collector
}

// Results represents the analysis results
type Results struct {
	DemoStats  *stats.DemoStats
	Categories []stats.Category
}

// NewAnalyzer creates a new analyzer for the given demo file
func NewAnalyzer(demoPath string) *Analyzer {
	analyzer := &Analyzer{
		demoPath:   demoPath,
		collectors: []stats.Collector{},
	}

	// Register default collectors
	analyzer.RegisterCollector(stats.NewWeaponUsageCollector())
	analyzer.RegisterCollector(stats.NewHeadshotCollector())
	analyzer.RegisterCollector(stats.NewSnapAngleCollector())
	analyzer.RegisterCollector(stats.NewReactionTimeCollector())
	analyzer.RegisterCollector(stats.NewRecoilControlCollector()) // Add the new recoil control collector
	analyzer.RegisterCollector(stats.NewGameModeCollector())      // Add the game mode collector
	analyzer.RegisterCollector(stats.NewCheatDetector())          // CheatDetector should be last to use results from other collectors

	return analyzer
}

// RegisterCollector adds a new statistics collector to the analyzer
func (a *Analyzer) RegisterCollector(collector stats.Collector) {
	a.collectors = append(a.collectors, collector)
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
	header, err := parser.ParseHeader()
	if err != nil {
		return Results{}, fmt.Errorf("failed to parse demo header: %w", err)
	}

	// Initialize demo stats
	demoStats := stats.NewDemoStats()
	demoStats.TickRate = parser.TickRate()
	demoStats.DemoName = filepath.Base(a.demoPath)
	demoStats.MapName = header.MapName

	// Set up collectors
	for _, collector := range a.collectors {
		collector.Setup(parser, demoStats)
	}

	// Parse all frames
	frameCount := 0
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

		// Collect stats for this frame
		for _, collector := range a.collectors {
			collector.CollectFrame(parser, demoStats)
		}

		frameCount++
	}

	// Store total frames parsed
	demoStats.TickCount = frameCount

	// Calculate final stats
	for _, collector := range a.collectors {
		collector.CollectFinalStats(demoStats)
	}

	// Collect categories from all collectors
	categories := make([]stats.Category, 0)
	categoriesSet := make(map[stats.Category]bool)

	for _, collector := range a.collectors {
		for _, category := range collector.Categories() {
			if !categoriesSet[category] {
				categories = append(categories, category)
				categoriesSet[category] = true
			}
		}
	}

	return Results{
		DemoStats:  demoStats,
		Categories: categories,
	}, nil
}
