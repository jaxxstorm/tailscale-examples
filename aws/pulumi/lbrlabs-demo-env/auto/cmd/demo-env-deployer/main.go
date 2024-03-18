package main

import (
	"os"

	"github.com/spf13/viper"

	"github.com/jaxxstorm/tailscale-examples/aws/pulumi/lbrlabs-demo-env/auto/cmd/demo-env-deployer/version"
	"github.com/spf13/cobra"
)

var (
	profile string
)

func configureCLI() *cobra.Command {
	rootCommand := &cobra.Command{
		Use:  "demo-env-deployer",
		Long: "",
	}

	rootCommand.AddCommand(version.Command())

	return rootCommand
}

func main() {
	rootCommand := configureCLI()

	if err := rootCommand.Execute(); err != nil {
		os.Exit(1)
	}
}