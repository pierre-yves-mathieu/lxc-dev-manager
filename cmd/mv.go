package cmd

import (
	"fmt"
	"os"
	"path"
	"strings"

	"lxc-dev-manager/internal/lxc"

	"github.com/spf13/cobra"
)

var mvCmd = &cobra.Command{
	Use:   "mv <source> <container>:<dest>",
	Short: "Copy file or folder from host to container",
	Long: `Copy a file or directory from the host to a container.

The destination must be in the format container:path.

Examples:
  lxc-dev-manager mv ./app dev1:/home/dev/app
  lxc-dev-manager mv config.json dev1:/etc/myapp/
  lxc-dev-manager mv ./project dev1:/home/dev/
  lxc-dev-manager mv ./data dev1:/opt/data -y  # auto-create directory`,
	Args: cobra.ExactArgs(2),
	RunE: runMv,
}

var mvYes bool

func init() {
	rootCmd.AddCommand(mvCmd)
	mvCmd.Flags().BoolVarP(&mvYes, "yes", "y", false, "Auto-create destination directory if it doesn't exist")
}

func runMv(cmd *cobra.Command, args []string) error {
	source := args[0]
	dest := args[1]

	// Parse destination: container:path
	parts := strings.SplitN(dest, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid destination format. Use container:path (e.g., dev1:/home/dev/)")
	}
	containerName := parts[0]
	remotePath := parts[1]

	if remotePath == "" {
		return fmt.Errorf("destination path cannot be empty")
	}

	// Validate source exists
	info, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("source '%s' does not exist", source)
		}
		return fmt.Errorf("cannot access source '%s': %w", source, err)
	}

	// Validate container exists
	cfg, lxcName, err := requireContainer(containerName)
	if err != nil {
		return err
	}

	// Expand ~ to user's home directory
	if strings.HasPrefix(remotePath, "~/") {
		user := cfg.GetUser(containerName)
		remotePath = "/home/" + user.Name + remotePath[1:]
	} else if remotePath == "~" {
		user := cfg.GetUser(containerName)
		remotePath = "/home/" + user.Name
	}

	// Determine if recursive (directory)
	recursive := info.IsDir()

	// Get the destination directory to check/create
	// For files: parent of the file path
	// For directories: parent of the destination (since lxc file push -r copies INTO dest)
	destDir := path.Dir(remotePath)

	// Check if destination directory exists
	user := cfg.GetUser(containerName)
	if !lxc.DirExists(lxcName, destDir) {
		if !mvYes && !confirmPrompt(fmt.Sprintf("Directory '%s' does not exist. Create it?", destDir)) {
			return fmt.Errorf("destination directory does not exist")
		}
		fmt.Printf("Creating directory '%s'...\n", destDir)
		if err := lxc.Exec(lxcName, "mkdir", "-p", destDir); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		// Set ownership on created directory
		lxc.Exec(lxcName, "chown", user.Name+":"+user.Name, destDir)
	}

	// Push the file
	// For directories, lxc file push -r copies INTO the destination,
	// so we push to the parent directory
	pushPath := remotePath
	if recursive {
		pushPath = path.Dir(remotePath)
		fmt.Printf("Copying directory '%s' to %s:%s...\n", source, containerName, remotePath)
	} else {
		fmt.Printf("Copying file '%s' to %s:%s...\n", source, containerName, remotePath)
	}

	if err := lxc.FilePush(lxcName, source, pushPath, recursive); err != nil {
		return err
	}

	// Fix ownership to the container's configured user
	if recursive {
		if err := lxc.Exec(lxcName, "chown", "-R", user.Name+":"+user.Name, remotePath); err != nil {
			fmt.Printf("Warning: could not set ownership: %v\n", err)
		}
	} else {
		if err := lxc.Exec(lxcName, "chown", user.Name+":"+user.Name, remotePath); err != nil {
			fmt.Printf("Warning: could not set ownership: %v\n", err)
		}
	}

	fmt.Println("Done.")
	return nil
}
