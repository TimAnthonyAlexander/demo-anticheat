package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/timanthonyalexander/demo-anticheat/pkg/analyzer"
	"github.com/timanthonyalexander/demo-anticheat/pkg/stats"
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

		// Create a reporter
		reporter := stats.NewTextReporter("CS2 Demo Analysis Results")

		// Generate the report
		fmt.Println("Analysis complete!")
		err = reporter.Report(results.DemoStats, results.Categories, os.Stdout)
		if err != nil {
			return fmt.Errorf("error generating report: %v", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
}
