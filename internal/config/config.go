package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"lxc-dev-manager/internal/validation"

	"gopkg.in/yaml.v3"
)

const (
	ConfigFile  = "containers.yaml"
	lockFile    = "containers.yaml.lock"
	lockTimeout = 5 * time.Second
)

type Config struct {
	Project    string               `yaml:"project"`
	Defaults   Defaults             `yaml:"defaults"`
	Containers map[string]Container `yaml:"containers"`
}

type User struct {
	Name     string `yaml:"name,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type Defaults struct {
	Ports []int `yaml:"ports"`
	User  User  `yaml:"user,omitempty"`
}

type Snapshot struct {
	Description string `yaml:"description,omitempty"`
	CreatedAt   string `yaml:"created_at"`
}

type Container struct {
	Image     string              `yaml:"image"`
	Ports     []int               `yaml:"ports,omitempty"`
	User      User                `yaml:"user,omitempty"`
	Snapshots map[string]Snapshot `yaml:"snapshots,omitempty"`
}

func Load() (*Config, error) {
	data, err := os.ReadFile(ConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return nil if file doesn't exist - project must be explicitly created
			return nil, nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", ConfigFile, err)
	}

	if cfg.Containers == nil {
		cfg.Containers = make(map[string]Container)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks all configuration values for correctness
func (c *Config) Validate() error {
	// Validate project name
	if c.Project != "" && !IsValidProjectName(c.Project) {
		return fmt.Errorf("invalid project name %q", c.Project)
	}

	// Validate default ports
	if err := validation.ValidatePorts(c.Defaults.Ports); err != nil {
		return fmt.Errorf("invalid default ports: %w", err)
	}

	// Validate each container
	for name, container := range c.Containers {
		if err := validation.ValidateFullContainerName(c.Project, name); err != nil {
			return fmt.Errorf("container '%s': %w", name, err)
		}

		if len(container.Ports) > 0 {
			if err := validation.ValidatePorts(container.Ports); err != nil {
				return fmt.Errorf("container '%s': %w", name, err)
			}
		}
	}

	return nil
}

// GetLXCName returns the full LXC container name with project prefix
func (c *Config) GetLXCName(shortName string) string {
	if c.Project == "" {
		return shortName
	}
	return c.Project + "-" + shortName
}

// GetShortName extracts short name from LXC name by stripping project prefix
func (c *Config) GetShortName(lxcName string) string {
	if c.Project == "" {
		return lxcName
	}
	prefix := c.Project + "-"
	if strings.HasPrefix(lxcName, prefix) {
		return strings.TrimPrefix(lxcName, prefix)
	}
	return lxcName
}

// HasProject returns true if project is initialized
func (c *Config) HasProject() bool {
	return c.Project != ""
}

// GetProjectFromFolder returns the current directory name
func GetProjectFromFolder() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Base(cwd), nil
}

// IsValidProjectName validates project name (alphanumeric, hyphens, underscores only)
func IsValidProjectName(name string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	return re.MatchString(name)
}

func (c *Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return atomicWriteFile(ConfigFile, data, 0644)
}

// atomicWriteFile writes data to a file atomically using temp file + rename.
// This prevents corruption from partial writes if the process is interrupted.
func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if dir == "" {
		dir = "."
	}

	tmp, err := os.CreateTemp(dir, ".containers.yaml.tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	success := false
	defer func() {
		if !success {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return nil
}

// ConfigLock represents an exclusive lock on the config file.
// Use this when performing Load→Modify→Save operations to prevent race conditions.
type ConfigLock struct {
	file *os.File
}

// AcquireLock acquires an exclusive lock on the config file with timeout.
func AcquireLock() (*ConfigLock, error) {
	lockPath := lockFile
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("timeout waiting for config lock (another instance may be running)")
		}
		time.Sleep(100 * time.Millisecond)
	}

	return &ConfigLock{file: f}, nil
}

// Release releases the config lock.
func (l *ConfigLock) Release() error {
	if l.file == nil {
		return nil
	}
	syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	err := l.file.Close()
	l.file = nil
	return err
}

// LoadWithLock loads the config while holding an exclusive lock.
// The caller must call Release() on the returned lock when done.
func LoadWithLock() (*Config, *ConfigLock, error) {
	lock, err := AcquireLock()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := Load()
	if err != nil {
		lock.Release()
		return nil, nil, err
	}

	return cfg, lock, nil
}

func (c *Config) AddContainer(name, image string) {
	c.Containers[name] = Container{
		Image: image,
	}
}

func (c *Config) RemoveContainer(name string) {
	delete(c.Containers, name)
}

func (c *Config) GetPorts(name string) []int {
	if container, ok := c.Containers[name]; ok && len(container.Ports) > 0 {
		return container.Ports
	}
	return c.Defaults.Ports
}

// GetUser returns the user config for a container (per-container > defaults > hardcoded)
func (c *Config) GetUser(name string) User {
	// Check per-container first
	if container, ok := c.Containers[name]; ok && container.User.Name != "" {
		user := container.User
		// Fill in missing password from defaults or hardcoded
		if user.Password == "" {
			user.Password = c.Defaults.User.Password
		}
		if user.Password == "" {
			user.Password = "dev"
		}
		return user
	}
	// Fall back to defaults
	if c.Defaults.User.Name != "" {
		user := c.Defaults.User
		if user.Password == "" {
			user.Password = "dev"
		}
		return user
	}
	// Hardcoded fallback
	return User{Name: "dev", Password: "dev"}
}

func (c *Config) HasContainer(name string) bool {
	_, ok := c.Containers[name]
	return ok
}

func (c *Config) AddSnapshot(containerName, snapshotName, description string) {
	container := c.Containers[containerName]
	if container.Snapshots == nil {
		container.Snapshots = make(map[string]Snapshot)
	}
	container.Snapshots[snapshotName] = Snapshot{
		Description: description,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}
	c.Containers[containerName] = container
}

func (c *Config) RemoveSnapshot(containerName, snapshotName string) {
	if container, ok := c.Containers[containerName]; ok {
		delete(container.Snapshots, snapshotName)
		c.Containers[containerName] = container
	}
}

func (c *Config) GetSnapshots(containerName string) map[string]Snapshot {
	if container, ok := c.Containers[containerName]; ok {
		return container.Snapshots
	}
	return nil
}

func (c *Config) HasSnapshot(containerName, snapshotName string) bool {
	if container, ok := c.Containers[containerName]; ok {
		_, exists := container.Snapshots[snapshotName]
		return exists
	}
	return false
}
