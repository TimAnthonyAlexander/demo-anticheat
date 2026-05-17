package stats

import (
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
)

// CheatDetector is the Collector facade for the cheat-detection scoring
// pipeline. All scoring logic lives in cheatscore_*.go files within this
// package so it can be unit-tested without spinning up a parser.
type CheatDetector struct {
	*BaseCollector
}

func NewCheatDetector() *CheatDetector {
	return &CheatDetector{
		BaseCollector: NewBaseCollector("Cheat Detection", Category("anti_cheat")),
	}
}

func (cd *CheatDetector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {}

func (cd *CheatDetector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {}

// CollectFinalStats delegates to cheatscoreEvaluate, which writes all
// anti_cheat metrics (cheat_likelihood, per-channel scores, boost flags,
// cheater Yes/No) into each player's PlayerStats.
func (cd *CheatDetector) CollectFinalStats(demoStats *DemoStats) {
	cheatscoreEvaluate(demoStats)
}
