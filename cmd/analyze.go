package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timanthonyalexander/demo-anticheat/pkg/analyzer"
	"github.com/timanthonyalexander/demo-anticheat/pkg/stats"
)

var htmlOut bool

const htmlEnvVar = "DEMOANTICHEAT_HTML"
const htmlOutputFile = "index.html"

var analyzeCmd = &cobra.Command{
	Use:   "analyze [demo-file]",
	Short: "Analyze a CS2 demo file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		demoPath := args[0]

		if _, err := os.Stat(demoPath); os.IsNotExist(err) {
			return fmt.Errorf("demo file not found: %s", demoPath)
		}
		if filepath.Ext(demoPath) != ".dem" {
			return fmt.Errorf("file must have .dem extension: %s", demoPath)
		}

		fmt.Printf("Analyzing demo file: %s\n", demoPath)

		demoAnalyzer := analyzer.NewAnalyzer(demoPath)

		fmt.Println("Analysis in progress...")
		results, err := demoAnalyzer.Analyze()
		if err != nil {
			return fmt.Errorf("analysis failed: %v", err)
		}

		reporter := stats.NewTextReporter("CS2 Demo Analysis Results")

		fmt.Println("Analysis complete!")
		if err := reporter.Report(results.DemoStats, results.Categories, os.Stdout); err != nil {
			return fmt.Errorf("error generating report: %v", err)
		}

		if shouldWriteHTML() {
			if err := writeHTMLReport(results); err != nil {
				return fmt.Errorf("error generating html report: %v", err)
			}
		}

		return nil
	},
}

func shouldWriteHTML() bool {
	if htmlOut {
		return true
	}
	return envTruthy(os.Getenv(htmlEnvVar))
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off", "f", "n":
		return false
	}
	return true
}

func writeHTMLReport(results analyzer.Results) error {
	reporter, err := stats.NewHTMLReporter()
	if err != nil {
		return err
	}

	f, err := os.Create(htmlOutputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := reporter.Report(results.DemoStats, results.Categories, f); err != nil {
		return err
	}

	abs, _ := filepath.Abs(htmlOutputFile)
	fmt.Printf("\nHTML report written to: %s\n", abs)
	return nil
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.Flags().BoolVar(&htmlOut, "html", false, "Also write an HTML report to ./index.html")
}
