# demo-anticheat

**CS2 Demo Automated Cheat Detection**  
_A fast, extensible CLI tool for statistically analyzing Counter-Strike 2 demos and surfacing likely cheaters with explainable metrics._

---

## Features

- **Automated parsing and statistical analysis of CS2 demo files**
- **Cheat detection based on real game data, not just intuition**
- **Metrics for every player:**
  - Weapon usage breakdown (knife, non-knife, unarmed)
  - Headshot percentage (with minimum sample guard)
  - Snap-angle velocity (degrees per millisecond, P95/avg/median)
  - Human reaction time window (10th percentile, median, sub-100ms shot ratio)
- **Composite cheat-likelihood score** with clear justification for every flag
- **CLI interface, rapid bulk processing, and machine-readable output possible**
- **Modular, extensible architecture** – add your own metrics with minimal code

---

## Getting Started

### Install

Requires Go ≥ 1.18

```sh
git clone https://github.com/timanthonyalexander/demo-anticheat
cd demo-anticheat
go build
```

### Analyze a Demo

```sh
./demo-anticheat analyze path/to/demo.dem
```

The tool will process the demo and print a multi-part report showing weapon, aim, reaction, and cheat-likelihood statistics for every player.

⸻

Extending With New Statistics

The system is designed for rapid experimentation:
	1.	Create a new collector by implementing the stats.Collector interface.
	2.	Register your collector with the analyzer (in pkg/analyzer/analyzer.go).
	3.	Your stats will be auto-included in the per-player and per-demo output.

Example: Adding a Custom Statistic

```
type MyStatsCollector struct {
    *stats.BaseCollector
}

func NewMyStatsCollector() *MyStatsCollector {
    return &MyStatsCollector{
        BaseCollector: stats.NewBaseCollector("My Stats", stats.Category("my_category")),
    }
}

func (c *MyStatsCollector) CollectFrame(parser demoinfocs.Parser, demoStats *stats.DemoStats) {
    // Frame-by-frame logic here
}

func (c *MyStatsCollector) CollectFinalStats(demoStats *stats.DemoStats) {
    // End-of-demo aggregation here
}
```

Register your collector with the analyzer so it’s included in every run.

⸻

### Statistics Included
	•	Weapon Usage:
Time spent with knife, non-knife, and no weapon.
	•	Headshot Percentage:
Per-player, with minimum kill filter.
	•	Snap Angle Velocity:
Measures speed of aim adjustments prior to a kill. Bots snap far faster than humanly possible.
	•	Reaction Time:
Time (ms) between enemy entering FOV and player’s shot. Flags superhuman trigger-bots.
	•	Composite Cheat Detection:
Weighted, explainable flagging based on headshot %, snap speed, and reaction time.

⸻

## Philosophy

demo-anticheat aims to provide objective, transparent, and extensible cheat detection for Counter-Strike 2 servers, leagues, or analysts.
It’s not a “VAC” clone – every verdict is backed by statistics you can read and adjust.
Add your own metrics, tune the weights, or use as a baseline for ML-based detection.

⸻

## Contributing

Pull requests and new metric ideas are welcome.
This project values modularity and clarity—add your own collector, document your math, and show your work.

⸻

## License

MIT

⸻

For feedback, bugs, or suggestions, open an issue or contact Tim at info@t17r.com

Let me know if you want a compact or more technical/contributor-focused version.
