package cmd

import (
	"fmt"
	"time"

	"lxc-dev-manager/internal/lxc"
	"lxc-dev-manager/internal/validation"

	"github.com/spf13/cobra"
)

var containerCmd = &cobra.Command{
	Use:     "container",
	Aliases: []string{"c"},
	Short:   "Manage containers within the project",
	Long: `Commands for managing containers within the current project.

All container names are prefixed with the project name in LXC.
For example, if the project is "webapp" and you create container "dev1",
the actual LXC container will be named "webapp-dev1".`,
}

var containerCreateCmd = &cobra.Command{
	Use:   "create <name> <image>",
	Short: "Create a new container in the current project",
	Long: `Create a new container from an image and configure it for development.

The container will be set up with:
  - Nesting enabled (Docker support)
  - User with passwordless sudo (configurable in containers.yaml, default: dev/dev)
  - SSH enabled

The container name will be prefixed with the project name in LXC.

Examples:
  lxc-dev-manager container create dev1 ubuntu:24.04
  lxc-dev-manager c create myapp my-custom-base`,
	Args: cobra.ExactArgs(2),
	RunE: runContainerCreate,
}

var containerResetCmd = &cobra.Command{
	Use:   "reset <container> [snapshot]",
	Short: "Reset container to a snapshot",
	Long: `Reset a container to a snapshot state.

If no snapshot is specified, resets to 'initial-state'.
Uses ZFS snapshots - the operation is instant.

Examples:
  lxc-dev-manager container reset dev1                    # reset to initial-state
  lxc-dev-manager container reset dev1 before-refactor    # reset to named snapshot`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runContainerReset,
}

var containerCloneCmd = &cobra.Command{
	Use:   "clone <source> <new-name>",
	Short: "Clone a container",
	Long: `Clone an existing container to create a new one.

By default, clones the current state of the container. Use --snapshot to clone
from a specific snapshot instead.

The cloned container will:
  - Have all the same data as the source
  - Get a new 'initial-state' snapshot
  - Be registered in the project config

Examples:
  lxc-dev-manager container clone dev dev2                     # clone current state
  lxc-dev-manager container clone dev dev2 --snapshot checkpoint  # clone from snapshot`,
	Args: cobra.ExactArgs(2),
	RunE: runContainerClone,
}

var cloneSnapshot string

func init() {
	rootCmd.AddCommand(containerCmd)
	containerCmd.AddCommand(containerCreateCmd)
	containerCmd.AddCommand(containerResetCmd)
	containerCmd.AddCommand(containerCloneCmd)

	// Clone flags
	containerCloneCmd.Flags().StringVarP(&cloneSnapshot, "snapshot", "s", "", "Clone from a specific snapshot instead of current state")
}

func runContainerCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	image := args[1]

	// Validate container name first
	if err := validation.ValidateContainerName(name); err != nil {
		return fmt.Errorf("invalid container name: %w", err)
	}

	// Load config with lock to prevent race conditions
	cfg, lock, err := requireProjectWithLock()
	if err != nil {
		return err
	}
	defer lock.Release()

	// Validate combined name (project + container)
	if err := validation.ValidateFullContainerName(cfg.Project, name); err != nil {
		return err
	}

	// Check if already exists in config
	if cfg.HasContainer(name) {
		return fmt.Errorf("container '%s' already exists in config", name)
	}

	// Get full LXC name with prefix
	lxcName := cfg.GetLXCName(name)

	// Check if already exists in LXC
	if lxc.Exists(lxcName) {
		return fmt.Errorf("container '%s' already exists in LXC", lxcName)
	}

	// Launch container
	fmt.Printf("Creating container '%s' (LXC: %s) from image '%s'...\n", name, lxcName, image)
	if err := lxc.Launch(lxcName, image); err != nil {
		return err
	}

	// Enable nesting for Docker support
	fmt.Println("Enabling nesting (Docker support)...")
	if err := lxc.EnableNesting(lxcName); err != nil {
		// Non-fatal, just warn
		fmt.Printf("Warning: could not enable nesting: %v\n", err)
	}

	// Wait for container to be ready
	fmt.Println("Waiting for container to be ready...")
	if err := lxc.WaitForReady(lxcName, 60*time.Second); err != nil {
		return err
	}

	// Get user config (per-container > defaults > hardcoded dev/dev)
	user := cfg.GetUser(name)

	// Set up user
	fmt.Printf("Setting up '%s' user...\n", user.Name)
	if err := lxc.SetupUser(lxcName, user.Name, user.Password); err != nil {
		return fmt.Errorf("failed to set up user: %w", err)
	}

	// Enable SSH
	fmt.Println("Enabling SSH...")
	if err := lxc.EnableSSH(lxcName); err != nil {
		return fmt.Errorf("failed to enable SSH: %w", err)
	}

	// Get IP
	ip, err := lxc.GetIP(lxcName)
	if err != nil {
		ip = "(pending)"
	}

	// Add to config with short name
	cfg.AddContainer(name, image)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Create initial snapshot for reset (instant with ZFS)
	fmt.Println("Creating initial state snapshot...")
	if err := lxc.Snapshot(lxcName, "initial-state"); err != nil {
		fmt.Printf("Warning: could not create initial snapshot: %v\n", err)
	} else {
		// Register in config
		cfg.AddSnapshot(name, "initial-state", "Initial state after setup")
		cfg.Save()
	}

	fmt.Printf("\nContainer '%s' created successfully!\n", name)
	fmt.Printf("  LXC name: %s\n", lxcName)
	fmt.Printf("  IP: %s\n", ip)
	fmt.Printf("  User: %s / Password: %s\n", user.Name, user.Password)
	fmt.Printf("\nConnect with: lxc-dev-manager ssh %s\n", name)

	return nil
}

func runContainerReset(cmd *cobra.Command, args []string) error {
	name := args[0]
	snapshotName := "initial-state"
	if len(args) > 1 {
		snapshotName = args[1]
	}

	_, lxcName, err := requireContainer(name)
	if err != nil {
		return err
	}

	// Check if snapshot exists
	if !lxc.SnapshotExists(lxcName, snapshotName) {
		if snapshotName == "initial-state" {
			return fmt.Errorf("container '%s' has no initial-state snapshot (created before this feature was added)", name)
		}
		return fmt.Errorf("snapshot '%s' does not exist", snapshotName)
	}

	// Check if running
	status, err := lxc.GetStatus(lxcName)
	if err != nil {
		return err
	}
	wasRunning := status == "RUNNING"

	// Stop if running
	if wasRunning {
		fmt.Printf("Stopping container '%s'...\n", name)
		if err := lxc.Stop(lxcName); err != nil {
			return err
		}
	}

	// Restore from snapshot
	fmt.Printf("Restoring container '%s' to snapshot '%s'...\n", name, snapshotName)
	if err := lxc.Restore(lxcName, snapshotName); err != nil {
		return err
	}

	// Restart if was running
	if wasRunning {
		fmt.Printf("Starting container '%s'...\n", name)
		if err := lxc.Start(lxcName); err != nil {
			return err
		}

		// Get new IP
		ip, _ := lxc.GetIP(lxcName)
		if ip != "" {
			fmt.Printf("\nContainer '%s' reset to '%s' successfully! IP: %s\n", name, snapshotName, ip)
		} else {
			fmt.Printf("\nContainer '%s' reset to '%s' successfully!\n", name, snapshotName)
		}
	} else {
		fmt.Printf("\nContainer '%s' reset to '%s' successfully! (kept stopped)\n", name, snapshotName)
	}

	return nil
}

func runContainerClone(cmd *cobra.Command, args []string) error {
	sourceName := args[0]
	newName := args[1]

	// Validate new container name first
	if err := validation.ValidateContainerName(newName); err != nil {
		return fmt.Errorf("invalid container name: %w", err)
	}

	// Load config with lock to prevent race conditions
	cfg, sourceLXC, lock, err := requireContainerWithLock(sourceName)
	if err != nil {
		return err
	}
	defer lock.Release()

	// Validate combined name (project + container)
	if err := validation.ValidateFullContainerName(cfg.Project, newName); err != nil {
		return err
	}

	// Check if new name already exists
	if cfg.HasContainer(newName) {
		return fmt.Errorf("container '%s' already exists in config", newName)
	}

	newLXC := cfg.GetLXCName(newName)

	// Check if new name already exists in LXC
	if lxc.Exists(newLXC) {
		return fmt.Errorf("container '%s' already exists in LXC", newLXC)
	}

	// If cloning from snapshot, verify it exists
	if cloneSnapshot != "" {
		if !lxc.SnapshotExists(sourceLXC, cloneSnapshot) {
			return fmt.Errorf("snapshot '%s' does not exist on container '%s'", cloneSnapshot, sourceName)
		}
	}

	// Perform the clone
	if cloneSnapshot != "" {
		fmt.Printf("Cloning container '%s' (snapshot: %s) to '%s'...\n", sourceName, cloneSnapshot, newName)
		if err := lxc.CopySnapshot(sourceLXC, cloneSnapshot, newLXC); err != nil {
			return err
		}
	} else {
		fmt.Printf("Cloning container '%s' to '%s'...\n", sourceName, newName)
		if err := lxc.Copy(sourceLXC, newLXC); err != nil {
			return err
		}
	}

	// Get source container config to copy image info
	sourceImage := "cloned"
	if sourceContainer, ok := cfg.Containers[sourceName]; ok {
		sourceImage = sourceContainer.Image
	}

	// Add to config
	cfg.AddContainer(newName, sourceImage+":cloned-from-"+sourceName)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Create initial snapshot for reset
	fmt.Println("Creating initial state snapshot...")
	if err := lxc.Snapshot(newLXC, "initial-state"); err != nil {
		fmt.Printf("Warning: could not create initial snapshot: %v\n", err)
	} else {
		cfg.AddSnapshot(newName, "initial-state", "Initial state after clone")
		cfg.Save()
	}

	// Start the cloned container
	fmt.Println("Starting cloned container...")
	if err := lxc.Start(newLXC); err != nil {
		fmt.Printf("Warning: could not start container: %v\n", err)
	}

	// Get IP
	ip, _ := lxc.GetIP(newLXC)
	if ip == "" {
		ip = "(pending)"
	}

	// Get user config
	user := cfg.GetUser(newName)

	fmt.Printf("\nContainer '%s' cloned successfully!\n", newName)
	fmt.Printf("  LXC name: %s\n", newLXC)
	fmt.Printf("  Source: %s", sourceName)
	if cloneSnapshot != "" {
		fmt.Printf(" (snapshot: %s)", cloneSnapshot)
	}
	fmt.Println()
	fmt.Printf("  IP: %s\n", ip)
	fmt.Printf("  User: %s\n", user.Name)
	fmt.Printf("  SSH: ssh %s@%s\n", user.Name, ip)

	return nil
}
