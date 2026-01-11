package cmd

import (
	"fmt"

	"lxc-dev-manager/internal/lxc"

	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down <name>",
	Short: "Stop a container",
	Long: `Stop a running container.

Example:
  lxc-dev-manager down dev1`,
	Args: cobra.ExactArgs(1),
	RunE: runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, args []string) error {
	name := args[0]

	_, lxcName, err := requireContainer(name)
	if err != nil {
		return err
	}

	// Check current status
	status, err := lxc.GetStatus(lxcName)
	if err != nil {
		return err
	}

	if status == "STOPPED" {
		fmt.Printf("Container '%s' is already stopped\n", name)
		return nil
	}

	// Stop container
	fmt.Printf("Stopping container '%s'...\n", name)
	if err := lxc.Stop(lxcName); err != nil {
		return err
	}

	fmt.Printf("Container '%s' stopped\n", name)
	return nil
}
