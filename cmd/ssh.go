package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <name>",
	Short: "Open a shell in a container",
	Long: `Open an interactive bash shell in a container using lxc exec.

By default, logs in as the user defined in containers.yaml (defaults to 'dev').
Use -u to override with a different user, or -u root for root shell.

This is simpler than SSH and doesn't require network access.

Example:
  lxc-dev-manager ssh dev1          # Login as configured user
  lxc-dev-manager ssh dev1 -u root  # Login as root`,
	Args: cobra.ExactArgs(1),
	RunE: runSSH,
}

var sshUser string

func init() {
	rootCmd.AddCommand(sshCmd)
	sshCmd.Flags().StringVarP(&sshUser, "user", "u", "", "Override user (e.g., -u root for root shell)")
}

// buildSSHArgs constructs the lxc exec arguments for SSH
func buildSSHArgs(lxcName, user string) []string {
	args := []string{"exec", lxcName, "--"}

	if user != "" {
		// Use su -l to get a proper login shell with all supplementary groups loaded
		// This triggers PAM and loads groups from /etc/group (e.g., docker group)
		args = append(args, "su", "-l", user)
	} else {
		// Root shell
		args = append(args, "bash", "-l")
	}

	return args
}

func runSSH(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, lxcName, err := requireRunningContainer(name)
	if err != nil {
		return err
	}

	// Determine which user to use
	user := sshUser
	if cmd == nil || !cmd.Flags().Changed("user") {
		// No -u flag provided, use config user
		user = cfg.GetUser(name).Name
	}

	// Build lxc exec command
	lxcArgs := buildSSHArgs(lxcName, user)

	// Replace current process with lxc exec (interactive shell)
	lxcPath, err := exec.LookPath("lxc")
	if err != nil {
		return fmt.Errorf("lxc command not found: %w", err)
	}

	// Use syscall.Exec to replace the process for proper TTY handling
	return syscall.Exec(lxcPath, append([]string{"lxc"}, lxcArgs...), os.Environ())
}
