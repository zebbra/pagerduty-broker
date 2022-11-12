package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
}

var rootCmd = &cobra.Command{
	Use:           "pagerduty-broker [command]",
	Short:         "PagerDuty Broker",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
