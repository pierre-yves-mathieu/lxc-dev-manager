package cmd

import (
	"fmt"
	"strings"

	"lxc-dev-manager/internal/lxc"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all containers",
	Long: `List all containers defined in the config with their status.

Example:
  lxc-dev-manager list`,
	Args: cobra.NoArgs,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := requireProject()
	if err != nil {
		return err
	}

	// Show project header
	fmt.Printf("Project: %s\n\n", cfg.Project)

	if len(cfg.Containers) == 0 {
		fmt.Println("No containers defined in config")
		fmt.Println("Create one with: lxc-dev-manager container create <name> <image>")
		return nil
	}

	// Get all LXC container info
	lxcContainers, err := lxc.ListAll()
	if err != nil {
		return err
	}

	// Build lookup map
	lxcInfo := make(map[string]lxc.ContainerInfo)
	for _, c := range lxcContainers {
		lxcInfo[c.Name] = c
	}

	// Print header
	fmt.Printf("%-15s %-20s %-10s %-15s %s\n", "NAME", "IMAGE", "STATUS", "IP", "PORTS")
	fmt.Println(strings.Repeat("-", 75))

	// Print each container from config
	for name, container := range cfg.Containers {
		// Get full LXC name with prefix
		lxcName := cfg.GetLXCName(name)

		status := "NOT FOUND"
		ip := "-"

		if info, ok := lxcInfo[lxcName]; ok {
			status = info.Status
			if info.IP != "" {
				ip = info.IP
			}
		}

		ports := cfg.GetPorts(name)
		portStr := formatPorts(ports)

		// Display SHORT name, not LXC name
		fmt.Printf("%-15s %-20s %-10s %-15s %s\n", name, container.Image, status, ip, portStr)
	}

	return nil
}

func formatPorts(ports []int) string {
	if len(ports) == 0 {
		return "-"
	}

	strs := make([]string, len(ports))
	for i, p := range ports {
		strs[i] = fmt.Sprintf("%d", p)
	}
	return strings.Join(strs, ",")
}
