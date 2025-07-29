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

- **Weapon Usage Analysis**
    - Real-time tracking of equipment choices (knife, primary, secondary weapons)
    - Percentage breakdowns of combat vs. utility time
    - Weapon preference patterns that may indicate script assistance

- **Headshot Analytics**
    - Per-player headshot percentage with statistical significance guards
    - Headshot consistency across weapon types and engagement distances
    - Anomaly detection for headshot rates exceeding statistical norms

- **Precision Aim Detection**
    - Snap-angle velocity measurements (degrees per millisecond)
    - P95/median/average aim adjustment speed analysis
    - Sub-2° micro-adjustments tracking that identifies aim assistance

- **Reaction Time Analysis**
    - Human-impossible reaction windows flagging (sub-100ms response rates)
    - 10th percentile and median reaction time calculations
    - Visual contact to shot timing measurements across engagement types

- **Recoil Control Patterns**
    - Spray pattern deviation analysis against known weapon recoil
    - Inhuman consistency detection in automatic weapon control
    - Angular error measurements detecting script-assisted compensation

- **Game Context Intelligence**
    - Automatic game mode detection (Competitive, Wingman)
    - Round tracking and performance normalization
    - Special high-performance analysis for statistical outliers

- **Composite Cheat Detection**
    - Multi-factor weighted scoring system with transparent explanation
    - Customizable sensitivity with confidence thresholds
    - Performance-adjusted scoring that accounts for game context
    - Special flagging for exceptional performances (39+ kills in regulation, 15+ in Wingman)

Each metric is individually tracked, statistically validated, and contributes to an overall cheat likelihood score that provides clear justification for every verdict.

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
