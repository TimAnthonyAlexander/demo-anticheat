# CS2 Demo Statistics Tool

A command-line tool for analyzing Counter-Strike 2 demo files to generate statistics about gameplay.

## Features

- Analyze CS2 demo files (.dem format)
- Calculate weapon usage statistics (time spent with knife vs other weapons)

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

This will output statistics about how much time each player spent with their knife out versus other weapons.

## Example Output

```
Analyzing demo file: example.dem
Analysis in progress...
Analysis complete!

Weapon Usage Statistics (Knife vs Other Weapons):
------------------------------------------
Player                         Steam ID             Knife %      Other %     
------------------------------------------
player1                        76561198000000001   10.45        89.55       
player2                        76561198000000002   8.21         91.79       
player3                        76561198000000003   15.33        84.67       
player4                        76561198000000004   12.89        87.11       
player5                        76561198000000005   18.42        81.58       
```

## Project Structure

```
demo-anticheat/
├── cmd/                  # Command-line interface
│   ├── root.go           # Root command
│   └── analyze.go        # Analyze command
├── pkg/                  # Package code
│   └── analyzer/         # Demo analysis functionality
│       └── analyzer.go   # Core analysis logic
├── main.go               # Application entry point
├── go.mod                # Go module definition
└── README.md             # This file
```

## How It Works

1. The tool reads a CS2 demo file and parses it frame by frame
2. For each frame, it tracks what weapon each player is holding
3. It counts the number of ticks (frames) that players have their knives out versus other weapons
4. At the end, it calculates percentages to show how much time was spent with each weapon type

## License

MIT 