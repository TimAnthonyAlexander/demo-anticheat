package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/timanthonyalexander/demo-anticheat/pkg/analyzer"
	"github.com/timanthonyalexander/demo-anticheat/pkg/demo"
	"github.com/timanthonyalexander/demo-anticheat/pkg/stats"
)

var (
	outputDir      string
	keepDownloaded bool
)

// validateShareCode returns true if the input is a valid CS2 share code
func isShareCode(code string) bool {
	// CS2 share code pattern: CSGO-XXXXX-XXXXX-XXXXX-XXXXX-XXXXX
	// where X is alphanumeric
	shareCodePattern := regexp.MustCompile(`^CSGO-[A-Za-z0-9]{5}-[A-Za-z0-9]{5}-[A-Za-z0-9]{5}-[A-Za-z0-9]{5}-[A-Za-z0-9]{5}$`)
	return shareCodePattern.MatchString(code)
}

// analyzeCmd represents the analyze command
var analyzeCmd = &cobra.Command{
	Use:   "analyze [file_or_sharecode]",
	Short: "Analyze a CS2 demo file or share code",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		input := args[0]
		var demoPath string

		// Check if the input is a share code or local file
		if isShareCode(input) {
			fmt.Printf("Detected share code: %s\n", input)
			fmt.Println("Downloading demo file...")
			var err error
			demoPath, err = demo.DownloadFromShareCode(input, outputDir)
			if err != nil {
				return fmt.Errorf("failed to download demo: %v", err)
			}

			fmt.Printf("Demo downloaded to: %s\n", demoPath)

			// Clean up the file after analysis if not keeping
			if !keepDownloaded {
				defer os.Remove(demoPath)
			}
		} else {
			// Treat as local file path
			demoPath = input

			// Validate that the file exists
			if _, err := os.Stat(demoPath); os.IsNotExist(err) {
				return fmt.Errorf("demo file not found: %s", demoPath)
			}

			// Validate file extension
			if filepath.Ext(demoPath) != ".dem" {
				return fmt.Errorf("file must have .dem extension: %s", demoPath)
			}
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

	// Add flags for share code functionality
	analyzeCmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "Directory to save downloaded demo files (default: temporary directory)")
	analyzeCmd.Flags().BoolVarP(&keepDownloaded, "keep", "k", false, "Keep downloaded demo files after analysis")
}
