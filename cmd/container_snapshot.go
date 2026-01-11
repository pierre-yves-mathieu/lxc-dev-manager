package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"lxc-dev-manager/internal/lxc"

	"github.com/spf13/cobra"
)

var snapshotDescription string

var containerSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage container snapshots",
}

var containerSnapshotCreateCmd = &cobra.Command{
	Use:   "create <container> <name>",
	Short: "Create a named snapshot",
	Long: `Create a named snapshot of a container.

The snapshot is instant with ZFS storage.

Examples:
  lxc-dev-manager container snapshot create dev1 before-refactor
  lxc-dev-manager container snapshot create dev1 checkpoint -d "Before database migration"`,
	Args: cobra.ExactArgs(2),
	RunE: runSnapshotCreate,
}

var containerSnapshotListCmd = &cobra.Command{
	Use:   "list <container>",
	Short: "List snapshots for a container",
	Args:  cobra.ExactArgs(1),
	RunE:  runSnapshotList,
}

var containerSnapshotDeleteCmd = &cobra.Command{
	Use:   "delete <container> <name>",
	Short: "Delete a snapshot",
	Args:  cobra.ExactArgs(2),
	RunE:  runSnapshotDelete,
}

func init() {
	containerCmd.AddCommand(containerSnapshotCmd)
	containerSnapshotCmd.AddCommand(containerSnapshotCreateCmd)
	containerSnapshotCmd.AddCommand(containerSnapshotListCmd)
	containerSnapshotCmd.AddCommand(containerSnapshotDeleteCmd)

	containerSnapshotCreateCmd.Flags().StringVarP(&snapshotDescription, "description", "d", "", "Snapshot description")
}

func runSnapshotCreate(cmd *cobra.Command, args []string) error {
	containerName := args[0]
	snapshotName := args[1]

	// Load config with lock to prevent race conditions
	cfg, lxcName, lock, err := requireContainerWithLock(containerName)
	if err != nil {
		return err
	}
	defer lock.Release()

	// Check if snapshot already exists
	if lxc.SnapshotExists(lxcName, snapshotName) {
		return fmt.Errorf("snapshot '%s' already exists", snapshotName)
	}

	fmt.Printf("Creating snapshot '%s'...\n", snapshotName)
	if err := lxc.Snapshot(lxcName, snapshotName); err != nil {
		return err
	}

	// Register in config
	cfg.AddSnapshot(containerName, snapshotName, snapshotDescription)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Snapshot '%s' created successfully!\n", snapshotName)
	return nil
}

func runSnapshotList(cmd *cobra.Command, args []string) error {
	containerName := args[0]

	cfg, lxcName, err := requireContainer(containerName)
	if err != nil {
		return err
	}

	// Get snapshots from LXC
	lxcSnapshots, err := lxc.ListSnapshots(lxcName)
	if err != nil {
		return err
	}

	if len(lxcSnapshots) == 0 {
		fmt.Println("No snapshots found.")
		return nil
	}

	// Get metadata from config
	configSnapshots := cfg.GetSnapshots(containerName)

	// Sort snapshots by name
	sort.Strings(lxcSnapshots)

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCREATED\tDESCRIPTION")

	for _, name := range lxcSnapshots {
		created := "-"
		description := "-"
		if configSnapshots != nil {
			if meta, ok := configSnapshots[name]; ok {
				if meta.CreatedAt != "" {
					// Parse and format nicely
					t, err := time.Parse(time.RFC3339, meta.CreatedAt)
					if err == nil {
						created = t.Format("2006-01-02 15:04")
					}
				}
				if meta.Description != "" {
					description = meta.Description
				}
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", name, created, description)
	}
	w.Flush()

	return nil
}

func runSnapshotDelete(cmd *cobra.Command, args []string) error {
	containerName := args[0]
	snapshotName := args[1]

	// Load config with lock to prevent race conditions
	cfg, lxcName, lock, err := requireContainerWithLock(containerName)
	if err != nil {
		return err
	}
	defer lock.Release()

	// Prevent deleting initial-state
	if snapshotName == "initial-state" {
		return fmt.Errorf("cannot delete 'initial-state' snapshot (use container remove to delete container)")
	}

	if !lxc.SnapshotExists(lxcName, snapshotName) {
		return fmt.Errorf("snapshot '%s' does not exist", snapshotName)
	}

	fmt.Printf("Deleting snapshot '%s'...\n", snapshotName)
	if err := lxc.DeleteSnapshot(lxcName, snapshotName); err != nil {
		return err
	}

	// Remove from config
	cfg.RemoveSnapshot(containerName, snapshotName)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Snapshot '%s' deleted.\n", snapshotName)
	return nil
}
