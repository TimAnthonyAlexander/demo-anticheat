# CS2 Demo Statistics Tool

A command-line tool for analyzing Counter-Strike 2 demo files to generate statistics about gameplay.

## Features

- Analyze CS2 demo files (.dem format)
- Modular statistics collection system
- Extensible framework for adding new statistics
- Current statistics:
  - Weapon usage (knife vs other weapons)

## Installation

```bash
go install github.com/timanthonyalexander/demo-anticheat@latest
```

Or build from source:

```bash
git clone https://github.com/timanthonyalexander/demo-anticheat.git
cd demo-anticheat
go build -o demo-anticheat
```

## Usage

Analyze a demo file:

```bash
./demo-anticheat analyze path/to/demo.dem
```

This will output statistics about player behavior in the demo file.

## Example Output

```
CS2 Demo Analysis Results
Demo: example.dem
Map: de_dust2

=== Weapons Statistics ===

Player                          Steam ID               Knife          Non Knife    
-----------------------------------------------------------------------
player1                         76561198000000001      10.45%         89.55%       
player2                         76561198000000002      8.21%          91.79%       
player3                         76561198000000003      15.33%         84.67%       
player4                         76561198000000004      12.89%         87.11%       
player5                         76561198000000005      18.42%         81.58%       
```

## Project Structure

```
demo-anticheat/
├── cmd/                    # Command-line interface
│   ├── root.go             # Root command
│   └── analyze.go          # Analyze command
├── pkg/                    # Package code
│   ├── analyzer/           # Demo analysis orchestration
│   │   └── analyzer.go     # Core analysis logic
│   └── stats/              # Statistics collection system
│       ├── types.go        # Core types for statistics
│       ├── collectors.go   # Statistics collectors
│       └── reporters.go    # Output formatting
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
└── README.md               # This file
```

## How It Works

1. The analyzer parses the demo file frame by frame using demoinfocs-golang
2. Statistics collectors process each frame to gather data
3. After parsing, final statistics are calculated
4. A reporter formats and displays the statistics in a readable format

## Extending With New Statistics

The system is designed to be easily extensible:

1. Create a new collector by implementing the `stats.Collector` interface
2. Register your collector with the analyzer
3. Your statistics will be automatically included in the report

Example of implementing a new collector:

```go
type MyStatsCollector struct {
    *stats.BaseCollector
}

func NewMyStatsCollector() *MyStatsCollector {
    return &MyStatsCollector{
        BaseCollector: stats.NewBaseCollector("My Stats", stats.Category("my_category")),
    }
}

func (c *MyStatsCollector) CollectFrame(parser demoinfocs.Parser, demoStats *stats.DemoStats) {
    // Your frame-by-frame statistics logic here
}

func (c *MyStatsCollector) CollectFinalStats(demoStats *stats.DemoStats) {
    // Calculate final statistics here
}
```

## License

MIT 