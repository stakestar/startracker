package cli

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/stakestar/startracker/cli/node"
	"go.uber.org/zap"
)

var Logger *zap.Logger

var RootCmd = &cobra.Command{
	Use:   "startracker",
	Short: "startracker",
	Long:  `StarTracker is a CLI for running SSV nodes geo tracker`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
	},
}

func Execute(appName, version string) {
	RootCmd.Short = appName
	RootCmd.Version = version

	if err := RootCmd.Execute(); err != nil {
		log.Fatal("failed to execute root command", zap.Error(err))
	}
}

func init() {
	RootCmd.AddCommand(node.StartNodeCmd)
}
