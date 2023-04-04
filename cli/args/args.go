package args

import (
	"github.com/spf13/cobra"
)

type GlobalArgs struct {
	ConfigPath string
	LogLevel   string
}

func ProcessArgs(a *GlobalArgs, cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&a.ConfigPath, "config-path", "", "Config file path")
	_ = cmd.MarkPersistentFlagRequired("config-path")

	cmd.PersistentFlags().StringVarP(&a.LogLevel, "log-level", "l", "info", "Log level (debug, info, warn, error, fatal)")
}
