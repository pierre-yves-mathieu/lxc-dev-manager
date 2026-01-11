package validation

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// MaxContainerNameLength is the max length for a container name
	MaxContainerNameLength = 63
	// MaxCombinedLength is LXC's limit for full container name (project-container)
	MaxCombinedLength = 63
	// MinPort is the minimum valid port number
	MinPort = 1
	// MaxPort is the maximum valid port number
	MaxPort = 65535
	// PrivilegedPortMax is the highest port requiring root privileges
	PrivilegedPortMax = 1023
)

var (
	// LXC naming rules: start with letter, alphanumeric + hyphens
	containerNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]*$`)

	// Reserved names that conflict with LXC commands/concepts
	reservedNames = map[string]bool{
		"list":     true,
		"create":   true,
		"delete":   true,
		"start":    true,
		"stop":     true,
		"snapshot": true,
		"image":    true,
		"config":   true,
	}
)

// ValidateContainerName checks if a container name is valid for LXC
func ValidateContainerName(name string) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Errorf("container name cannot be empty")
	}

	if len(name) > MaxContainerNameLength {
		return fmt.Errorf("container name too long: %d characters (max %d)",
			len(name), MaxContainerNameLength)
	}

	if !containerNameRegex.MatchString(name) {
		if name[0] >= '0' && name[0] <= '9' {
			return fmt.Errorf("container name must start with a letter, not '%c'", name[0])
		}
		if strings.Contains(name, " ") {
			return fmt.Errorf("container name cannot contain spaces")
		}
		if strings.Contains(name, "_") {
			return fmt.Errorf("container name cannot contain underscores (use hyphens instead)")
		}
		return fmt.Errorf("container name contains invalid characters (allowed: letters, numbers, hyphens)")
	}

	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Errorf("container name cannot start or end with a hyphen")
	}

	if strings.Contains(name, "--") {
		return fmt.Errorf("container name cannot contain consecutive hyphens")
	}

	nameLower := strings.ToLower(name)
	if reservedNames[nameLower] {
		return fmt.Errorf("'%s' is a reserved name", name)
	}

	return nil
}

// ValidateFullContainerName checks if project + container name combination is valid
func ValidateFullContainerName(project, container string) error {
	if err := ValidateContainerName(container); err != nil {
		return err
	}

	fullName := container
	if project != "" {
		fullName = project + "-" + container
	}

	if len(fullName) > MaxCombinedLength {
		return fmt.Errorf("full container name '%s' too long: %d characters (max %d). "+
			"Use a shorter project or container name",
			fullName, len(fullName), MaxCombinedLength)
	}

	return nil
}

// ValidatePort checks if a port number is valid
func ValidatePort(port int) error {
	if port < MinPort || port > MaxPort {
		return fmt.Errorf("invalid port %d: must be between %d and %d",
			port, MinPort, MaxPort)
	}
	return nil
}

// ValidatePorts checks a list of ports
func ValidatePorts(ports []int) error {
	seen := make(map[int]bool)

	for _, port := range ports {
		if err := ValidatePort(port); err != nil {
			return err
		}

		if seen[port] {
			return fmt.Errorf("duplicate port %d in configuration", port)
		}
		seen[port] = true
	}

	return nil
}
