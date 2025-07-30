package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/timanthonyalexander/demo-anticheat/pkg/demo"
)

// shareCodeCmd represents the sharecode command
var shareCodeCmd = &cobra.Command{
	Use:   "sharecode [code]",
	Short: "Get information about a CS2 share code",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shareCode := args[0]

		// Validate share code
		if !isShareCode(shareCode) {
			return fmt.Errorf("invalid share code format: %s\nExpected format: CSGO-XXXXX-XXXXX-XXXXX-XXXXX-XXXXX", shareCode)
		}

		// Decode the share code
		match, outcome, token := demo.Decode(shareCode)
		url := demo.ReplayURL(shareCode)

		// Display information
		fmt.Println("CS2 Share Code Information")
		fmt.Println("=========================")
		fmt.Printf("Share Code: %s\n", shareCode)
		fmt.Printf("Match ID:   %021d\n", match)
		fmt.Printf("Outcome ID: %d\n", outcome)
		fmt.Printf("Token:      %d\n", token)
		fmt.Printf("Host:       %d\n", 128+int(outcome>>8&0xFF))
		fmt.Printf("Demo URL:   %s\n", url)
		fmt.Println()
		fmt.Println("To download and analyze this demo:")
		fmt.Printf("  ./demo-anticheat analyze %s\n", shareCode)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(shareCodeCmd)
}
