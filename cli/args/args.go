package args

import (
	"github.com/spf13/cobra"
)

type GlobalArgs struct {
	LogLevel string
}

func ProcessArgs(a *GlobalArgs, cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&a.LogLevel, "log-level", "l", "info", "Log level (debug, info, warn, error, fatal)")
}
