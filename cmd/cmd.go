package cmd

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var cmd = &cobra.Command{
	Use:   "cloudpipe",
	Short: "Cloudpipe demo",
	Long: `Cloudpipe broker

Simple runner for cloudpipe broker`,
}

const version = "0.1.0"

var log = logrus.WithFields(logrus.Fields{"version": version})

func init() {
	// TODO(vish): turn this into a global flag
	logrus.SetLevel(logrus.DebugLevel)
}

// Execute runs the base command
func Execute() {
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
