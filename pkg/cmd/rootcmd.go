package cmd

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootCommand returns a cobra command for mcpserver CLI tool
var rootCmd = &cobra.Command{
	Use:   "mcpserver",
	Short: "MCP Server for Genval and GenPod",
	Long: `
`,
	// SilenceErrors: true,
	SilenceUsage: true,
}

func init() {
	rootCmd.SetOut(color.Output)
	rootCmd.SetErr(color.Error)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
