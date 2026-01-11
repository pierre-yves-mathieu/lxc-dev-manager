package cmd

import (
	"fmt"
	"os"
	"time"

	"lxc-dev-manager/internal/lxc"

	"github.com/spf13/cobra"
)

var imageCreateCmd = &cobra.Command{
	Use:   "create <container> <image-name>",
	Short: "Create an image from a container",
	Long: `Create a reusable image from an existing container.

The container will be stopped before creating the image, then restarted.

Example:
  lxc-dev-manager image create dev1 my-base-image

Then create new containers from it:
  lxc-dev-manager container create dev2 my-base-image`,
	Args: cobra.ExactArgs(2),
	RunE: runImageCreate,
}

// imageCreateCmd is registered in image.go init()

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

func stepStart(step, total int, msg string) {
	fmt.Printf("%s[%d/%d]%s %s\n", colorCyan, step, total, colorReset, msg)
}

func stepDone(msg string) {
	fmt.Printf("      %s✓%s %s\n", colorGreen, colorReset, msg)
}

func stepInfo(msg string) {
	fmt.Printf("      %s\n", msg)
}

func runImageCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	imageName := args[1]
	snapshotName := fmt.Sprintf("snapshot-%d", time.Now().Unix())

	totalSteps := 4

	_, lxcName, err := requireContainer(name)
	if err != nil {
		return err
	}

	// Check if running, stop if so
	status, err := lxc.GetStatus(lxcName)
	if err != nil {
		return err
	}

	wasRunning := status == "RUNNING"

	// Step 1: Stop container
	stepStart(1, totalSteps, fmt.Sprintf("Stopping container '%s'...", name))
	if wasRunning {
		if err := lxc.Stop(lxcName); err != nil {
			return err
		}
		stepDone("Stopped")
	} else {
		stepDone("Already stopped")
	}

	// Step 2: Create snapshot (instant with ZFS/btrfs)
	stepStart(2, totalSteps, "Creating snapshot...")
	if err := lxc.Snapshot(lxcName, snapshotName); err != nil {
		return err
	}
	stepDone("Snapshot created (instant with ZFS)")

	// Step 3: Publish snapshot as image (this is the slow part)
	stepStart(3, totalSteps, fmt.Sprintf("Publishing image '%s'...", imageName))
	fmt.Println() // Extra line for LXC output

	// Create a prefixed writer to indent LXC output
	err = lxc.PublishSnapshotWithProgress(lxcName, snapshotName, imageName,
		&prefixWriter{prefix: "      ", w: os.Stdout},
		&prefixWriter{prefix: "      ", w: os.Stderr})

	// Clean up snapshot regardless of publish result
	lxc.DeleteSnapshot(lxcName, snapshotName)

	if err != nil {
		return err
	}
	fmt.Println()
	stepDone("Image published")

	// Step 4: Restart if was running
	stepStart(4, totalSteps, fmt.Sprintf("Restarting container '%s'...", name))
	if wasRunning {
		if err := lxc.Start(lxcName); err != nil {
			return fmt.Errorf("failed to restart container: %w", err)
		}
		// Get new IP
		ip, _ := lxc.GetIP(lxcName)
		if ip != "" {
			stepDone(fmt.Sprintf("Started (%s)", ip))
		} else {
			stepDone("Started")
		}
	} else {
		stepDone("Kept stopped (was not running before)")
	}

	fmt.Printf("\n%sImage '%s' created successfully!%s\n", colorGreen, imageName, colorReset)
	fmt.Printf("\nCreate new containers from it with:\n")
	fmt.Printf("  lxc-dev-manager container create <name> %s\n", imageName)

	return nil
}

// prefixWriter adds a prefix to each line of output
type prefixWriter struct {
	prefix     string
	w          *os.File
	needPrefix bool
}

func (pw *prefixWriter) Write(p []byte) (n int, err error) {
	if pw.needPrefix || len(p) == 0 {
		pw.w.WriteString(pw.prefix)
		pw.needPrefix = false
	}

	for i, b := range p {
		if b == '\n' && i < len(p)-1 {
			pw.w.Write(p[:i+1])
			pw.w.WriteString(pw.prefix)
			p = p[i+1:]
			i = -1
		}
	}

	if len(p) > 0 {
		pw.w.Write(p)
		if p[len(p)-1] == '\n' {
			pw.needPrefix = true
		}
	}

	return len(p), nil
}
