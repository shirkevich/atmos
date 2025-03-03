package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeStacksCmd describes configuration for stacks and components in the stacks
var describeStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Display configuration for Atmos stacks and their components",
	Long:               "This command shows the configuration details for Atmos stacks and the components within those stacks.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteDescribeStacksCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	describeStacksCmd.DisableFlagParsing = false

	describeStacksCmd.PersistentFlags().String("file", "", "Write the result to file: atmos describe stacks --file=stacks.yaml")

	describeStacksCmd.PersistentFlags().String("format", "yaml", "Specify the output format: atmos describe stacks --format=yaml|json ('yaml' is default)")

	describeStacksCmd.PersistentFlags().StringP("stack", "s", "",
		"Filter by a specific stack: atmos describe stacks -s <stack>\n"+
			"The filter supports names of the top-level stack manifests (including subfolder paths), and 'atmos' stack names (derived from the context vars)",
	)

	describeStacksCmd.PersistentFlags().String("components", "", "Filter by specific 'atmos' components: atmos describe stacks --components=<component1>,<component2>")

	describeStacksCmd.PersistentFlags().String("component-types", "", "Filter by specific component types: atmos describe stacks --component-types=terraform|helmfile. Supported component types: terraform, helmfile")

	describeStacksCmd.PersistentFlags().String("sections", "", "Output only the specified component sections: atmos describe stacks --sections=vars,settings. Available component sections: backend, backend_type, deps, env, inheritance, metadata, remote_state_backend, remote_state_backend_type, settings, vars")

	describeStacksCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command: atmos describe stacks --process-templates=false")

	describeStacksCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command: atmos describe stacks --process-functions=false")

	describeStacksCmd.PersistentFlags().Bool("include-empty-stacks", false, "Include stacks with no components in the output: atmos describe stacks --include-empty-stacks")

	describeStacksCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command: atmos describe stacks --skip=terraform.output")

	describeCmd.AddCommand(describeStacksCmd)
}
