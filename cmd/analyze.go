package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/timanthonyalexander/demo-anticheat/pkg/analyzer"

	"github.com/spf13/cobra"
)

// analyzeCmd represents the analyze command
var analyzeCmd = &cobra.Command{
	Use:   "analyze [file]",
	Short: "Analyze a CS2 demo file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		demoPath := args[0]

		// Validate that the file exists
		if _, err := os.Stat(demoPath); os.IsNotExist(err) {
			return fmt.Errorf("demo file not found: %s", demoPath)
		}

		// Validate file extension
		if filepath.Ext(demoPath) != ".dem" {
			return fmt.Errorf("file must have .dem extension: %s", demoPath)
		}

		fmt.Printf("Analyzing demo file: %s\n", demoPath)

		// Create an analyzer instance
		demoAnalyzer := analyzer.NewAnalyzer(demoPath)

		// Run the analysis
		fmt.Println("Analysis in progress...")
		results, err := demoAnalyzer.Analyze()
		if err != nil {
			return fmt.Errorf("analysis failed: %v", err)
		}

		// Display results
		fmt.Println("Analysis complete!")
		fmt.Println("\nWeapon Usage Statistics (Knife vs Other Weapons):")
		fmt.Println("------------------------------------------")
		fmt.Printf("%-30s %-20s %-12s %-12s\n", "Player", "Steam ID", "Knife %", "Other %")
		fmt.Println("------------------------------------------")

		for _, playerStats := range results.PlayerStats {
			fmt.Printf("%-30s %-20d %-12.2f %-12.2f\n",
				playerStats.PlayerName,
				playerStats.SteamID64,
				playerStats.KnifePercent,
				playerStats.NonKnifePercent,
			)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
}
