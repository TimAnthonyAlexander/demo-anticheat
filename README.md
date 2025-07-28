# CS2 Demo Statistics Tool

A command-line tool for analyzing Counter-Strike 2 demo files to generate statistics about gameplay and detect potential cheaters.

## Features

- Analyze CS2 demo files (.dem format)
- Modular statistics collection system
- Extensible framework for adding new statistics
- Anti-cheat detection system to identify potential cheaters
- Current statistics:
  - Weapon usage (knife vs other weapons)
  - Headshot kill percentage
  - Cheating likelihood

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

This will output statistics about player behavior in the demo file and estimate the likelihood of cheating.

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

=== Kills Statistics ===

Player                          Steam ID               Headshot       Total Kills  
-----------------------------------------------------------------------
player1                         76561198000000001      75.00%         12           
player2                         76561198000000002      33.33%         9            
player3                         76561198000000003      64.71%         17           
player4                         76561198000000004      28.57%         14           
player5                         76561198000000005      50.00%         10           

=== Anti Cheat Statistics ===

Player                          Steam ID               Cheat Likelihood
-----------------------------------------------------------------------
player1                         76561198000000001      78.42%         
player2                         76561198000000002      31.59%         
player3                         76561198000000003      65.34%         
player4                         76561198000000004      27.18%         
player5                         76561198000000005      52.75%         
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
│       ├── collectors.go   # Base collector functionality
│       ├── kill_collectors.go # Kill-related statistics
│       ├── cheat_detection.go # Anti-cheat detection system
│       └── reporters.go    # Output formatting
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
└── README.md               # This file
```

## How It Works

1. The analyzer parses the demo file frame by frame using demoinfocs-golang
2. Statistics collectors process each frame to gather data
3. After parsing, final statistics are calculated
4. The anti-cheat detection system analyzes collected statistics to estimate cheating likelihood
5. A reporter formats and displays the statistics in a readable format

### Implemented Statistics

#### Weapon Usage
Tracks the percentage of time players have their knife out versus other weapons throughout the demo.

#### Headshot Percentage
Tracks kills and headshot kills for each player, calculating the percentage of kills that were headshots.

#### Cheat Detection
Analyzes various statistical indicators to estimate the likelihood a player is cheating. Current factors include:

- Headshot percentage weighted by kill count (higher kills with high HS% is more suspicious)
- Unusual weapon usage patterns (very low or very high knife usage)

The detection system uses a weighted scoring model that will be expanded as more statistics are added.

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