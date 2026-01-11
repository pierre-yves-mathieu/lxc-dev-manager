package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Helper to run tests in a temp directory
func withTempDir(t *testing.T, fn func(dir string)) {
	t.Helper()
	dir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)
	fn(dir)
}

func TestLoad_FileNotExists(t *testing.T) {
	withTempDir(t, func(dir string) {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Should return nil - project must be explicitly created
		if cfg != nil {
			t.Fatal("expected nil config when file doesn't exist")
		}
	})
}

func TestLoad_ValidYAML(t *testing.T) {
	withTempDir(t, func(dir string) {
		yaml := `defaults:
  ports:
    - 3000
    - 8080
containers:
  dev1:
    image: ubuntu:24.04
  dev2:
    image: my-image
    ports:
      - 5000
`
		if err := os.WriteFile(ConfigFile, []byte(yaml), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(cfg.Defaults.Ports) != 2 {
			t.Errorf("expected 2 default ports, got %d", len(cfg.Defaults.Ports))
		}
		if cfg.Defaults.Ports[0] != 3000 {
			t.Errorf("expected port 3000, got %d", cfg.Defaults.Ports[0])
		}

		if len(cfg.Containers) != 2 {
			t.Errorf("expected 2 containers, got %d", len(cfg.Containers))
		}
		if cfg.Containers["dev1"].Image != "ubuntu:24.04" {
			t.Errorf("expected ubuntu:24.04, got %s", cfg.Containers["dev1"].Image)
		}
		if len(cfg.Containers["dev2"].Ports) != 1 {
			t.Errorf("expected 1 port for dev2, got %d", len(cfg.Containers["dev2"].Ports))
		}
	})
}

func TestLoad_InvalidYAML(t *testing.T) {
	withTempDir(t, func(dir string) {
		invalidYAML := `defaults:
  ports: [not valid yaml
    - broken
`
		if err := os.WriteFile(ConfigFile, []byte(invalidYAML), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid YAML, got nil")
		}
	})
}

func TestLoad_EmptyFile(t *testing.T) {
	withTempDir(t, func(dir string) {
		if err := os.WriteFile(ConfigFile, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Empty file should result in empty config with initialized map
		if cfg.Containers == nil {
			t.Error("expected Containers map to be initialized")
		}
	})
}

func TestSave_CreatesFile(t *testing.T) {
	withTempDir(t, func(dir string) {
		cfg := &Config{
			Defaults: Defaults{
				Ports: []int{5173, 8000},
			},
			Containers: map[string]Container{
				"test1": {Image: "ubuntu:24.04"},
			},
		}

		if err := cfg.Save(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(ConfigFile); os.IsNotExist(err) {
			t.Fatal("expected config file to be created")
		}

		// Verify content can be loaded back
		loaded, err := Load()
		if err != nil {
			t.Fatalf("failed to load saved config: %v", err)
		}
		if loaded.Containers["test1"].Image != "ubuntu:24.04" {
			t.Errorf("expected ubuntu:24.04, got %s", loaded.Containers["test1"].Image)
		}
	})
}

func TestSave_OverwritesFile(t *testing.T) {
	withTempDir(t, func(dir string) {
		// Create initial config
		cfg1 := &Config{
			Defaults:   Defaults{Ports: []int{3000}},
			Containers: map[string]Container{"old": {Image: "old-image"}},
		}
		if err := cfg1.Save(); err != nil {
			t.Fatal(err)
		}

		// Overwrite with new config
		cfg2 := &Config{
			Defaults:   Defaults{Ports: []int{8000}},
			Containers: map[string]Container{"new": {Image: "new-image"}},
		}
		if err := cfg2.Save(); err != nil {
			t.Fatal(err)
		}

		// Verify new content
		loaded, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := loaded.Containers["old"]; ok {
			t.Error("old container should not exist")
		}
		if loaded.Containers["new"].Image != "new-image" {
			t.Errorf("expected new-image, got %s", loaded.Containers["new"].Image)
		}
	})
}

func TestAddContainer(t *testing.T) {
	cfg := &Config{
		Containers: make(map[string]Container),
	}

	cfg.AddContainer("dev1", "ubuntu:24.04")

	if !cfg.HasContainer("dev1") {
		t.Error("expected dev1 to exist")
	}
	if cfg.Containers["dev1"].Image != "ubuntu:24.04" {
		t.Errorf("expected ubuntu:24.04, got %s", cfg.Containers["dev1"].Image)
	}
}

func TestAddContainer_Duplicate(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {Image: "old-image"},
		},
	}

	cfg.AddContainer("dev1", "new-image")

	if cfg.Containers["dev1"].Image != "new-image" {
		t.Errorf("expected new-image, got %s", cfg.Containers["dev1"].Image)
	}
}

func TestRemoveContainer(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu:24.04"},
			"dev2": {Image: "debian"},
		},
	}

	cfg.RemoveContainer("dev1")

	if cfg.HasContainer("dev1") {
		t.Error("dev1 should be removed")
	}
	if !cfg.HasContainer("dev2") {
		t.Error("dev2 should still exist")
	}
}

func TestRemoveContainer_NotExists(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{},
	}

	// Should not panic
	cfg.RemoveContainer("nonexistent")

	if len(cfg.Containers) != 0 {
		t.Error("containers should still be empty")
	}
}

func TestGetPorts_ContainerSpecific(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{Ports: []int{3000, 8000}},
		Containers: map[string]Container{
			"dev1": {
				Image: "ubuntu",
				Ports: []int{5000, 6000, 7000},
			},
		},
	}

	ports := cfg.GetPorts("dev1")

	if len(ports) != 3 {
		t.Errorf("expected 3 ports, got %d", len(ports))
	}
	if ports[0] != 5000 {
		t.Errorf("expected 5000, got %d", ports[0])
	}
}

func TestGetPorts_DefaultFallback(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{Ports: []int{3000, 8000}},
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu"}, // No ports specified
		},
	}

	ports := cfg.GetPorts("dev1")

	if len(ports) != 2 {
		t.Errorf("expected 2 default ports, got %d", len(ports))
	}
	if ports[0] != 3000 {
		t.Errorf("expected 3000, got %d", ports[0])
	}
}

func TestGetPorts_EmptyDefaults(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{Ports: []int{}},
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu"},
		},
	}

	ports := cfg.GetPorts("dev1")

	if len(ports) != 0 {
		t.Errorf("expected 0 ports, got %d", len(ports))
	}
}

func TestGetPorts_NonexistentContainer(t *testing.T) {
	cfg := &Config{
		Defaults:   Defaults{Ports: []int{3000}},
		Containers: map[string]Container{},
	}

	ports := cfg.GetPorts("nonexistent")

	// Should return defaults
	if len(ports) != 1 {
		t.Errorf("expected 1 port, got %d", len(ports))
	}
}

func TestHasContainer_Exists(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu"},
		},
	}

	if !cfg.HasContainer("dev1") {
		t.Error("expected HasContainer to return true")
	}
}

func TestHasContainer_NotExists(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{},
	}

	if cfg.HasContainer("dev1") {
		t.Error("expected HasContainer to return false")
	}
}

func TestLoad_PermissionError(t *testing.T) {
	withTempDir(t, func(dir string) {
		// Create unreadable file
		path := filepath.Join(dir, ConfigFile)
		if err := os.WriteFile(path, []byte("test"), 0000); err != nil {
			t.Skip("cannot create unreadable file")
		}
		defer os.Chmod(path, 0644) // Cleanup

		_, err := Load()
		if err == nil {
			t.Error("expected permission error")
		}
	})
}

// Snapshot tests

func TestAddSnapshot(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu:24.04"},
		},
	}

	cfg.AddSnapshot("dev1", "snap1", "Test snapshot")

	if !cfg.HasSnapshot("dev1", "snap1") {
		t.Error("expected snapshot to exist")
	}
	snap := cfg.Containers["dev1"].Snapshots["snap1"]
	if snap.Description != "Test snapshot" {
		t.Errorf("expected description 'Test snapshot', got '%s'", snap.Description)
	}
	if snap.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
}

func TestAddSnapshot_InitializesMap(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu:24.04"}, // No Snapshots map
		},
	}

	cfg.AddSnapshot("dev1", "snap1", "")

	if cfg.Containers["dev1"].Snapshots == nil {
		t.Error("expected Snapshots map to be initialized")
	}
}

func TestAddSnapshot_EmptyDescription(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu:24.04"},
		},
	}

	cfg.AddSnapshot("dev1", "snap1", "")

	snap := cfg.Containers["dev1"].Snapshots["snap1"]
	if snap.Description != "" {
		t.Errorf("expected empty description, got '%s'", snap.Description)
	}
}

func TestRemoveSnapshot(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {
				Image: "ubuntu:24.04",
				Snapshots: map[string]Snapshot{
					"snap1": {Description: "test"},
					"snap2": {Description: "test2"},
				},
			},
		},
	}

	cfg.RemoveSnapshot("dev1", "snap1")

	if cfg.HasSnapshot("dev1", "snap1") {
		t.Error("snap1 should be removed")
	}
	if !cfg.HasSnapshot("dev1", "snap2") {
		t.Error("snap2 should still exist")
	}
}

func TestRemoveSnapshot_NotExists(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu:24.04"},
		},
	}

	// Should not panic
	cfg.RemoveSnapshot("dev1", "nonexistent")
}

func TestRemoveSnapshot_ContainerNotExists(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{},
	}

	// Should not panic
	cfg.RemoveSnapshot("nonexistent", "snap1")
}

func TestGetSnapshots(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {
				Image: "ubuntu:24.04",
				Snapshots: map[string]Snapshot{
					"snap1": {Description: "test1"},
					"snap2": {Description: "test2"},
				},
			},
		},
	}

	snapshots := cfg.GetSnapshots("dev1")

	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snapshots))
	}
}

func TestGetSnapshots_ContainerNotExists(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{},
	}

	snapshots := cfg.GetSnapshots("nonexistent")

	if snapshots != nil {
		t.Error("expected nil for nonexistent container")
	}
}

func TestGetSnapshots_NoSnapshots(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu:24.04"},
		},
	}

	snapshots := cfg.GetSnapshots("dev1")

	if snapshots != nil {
		t.Error("expected nil when no snapshots")
	}
}

func TestHasSnapshot_True(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {
				Image: "ubuntu:24.04",
				Snapshots: map[string]Snapshot{
					"snap1": {Description: "test"},
				},
			},
		},
	}

	if !cfg.HasSnapshot("dev1", "snap1") {
		t.Error("expected HasSnapshot to return true")
	}
}

func TestHasSnapshot_False(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu:24.04"},
		},
	}

	if cfg.HasSnapshot("dev1", "snap1") {
		t.Error("expected HasSnapshot to return false")
	}
}

func TestHasSnapshot_ContainerNotExists(t *testing.T) {
	cfg := &Config{
		Containers: map[string]Container{},
	}

	if cfg.HasSnapshot("nonexistent", "snap1") {
		t.Error("expected HasSnapshot to return false")
	}
}

// User config tests

func TestGetUser_ContainerSpecific(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{User: User{Name: "default", Password: "defaultpass"}},
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu", User: User{Name: "alice", Password: "alicepass"}},
		},
	}

	user := cfg.GetUser("dev1")

	if user.Name != "alice" {
		t.Errorf("expected alice, got %s", user.Name)
	}
	if user.Password != "alicepass" {
		t.Errorf("expected alicepass, got %s", user.Password)
	}
}

func TestGetUser_DefaultFallback(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{User: User{Name: "default", Password: "defaultpass"}},
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu"}, // No user specified
		},
	}

	user := cfg.GetUser("dev1")

	if user.Name != "default" {
		t.Errorf("expected default, got %s", user.Name)
	}
	if user.Password != "defaultpass" {
		t.Errorf("expected defaultpass, got %s", user.Password)
	}
}

func TestGetUser_HardcodedFallback(t *testing.T) {
	cfg := &Config{
		Defaults:   Defaults{},
		Containers: map[string]Container{"dev1": {Image: "ubuntu"}},
	}

	user := cfg.GetUser("dev1")

	if user.Name != "dev" {
		t.Errorf("expected dev, got %s", user.Name)
	}
	if user.Password != "dev" {
		t.Errorf("expected dev, got %s", user.Password)
	}
}

func TestGetUser_NonexistentContainer(t *testing.T) {
	cfg := &Config{
		Defaults:   Defaults{User: User{Name: "default", Password: "pass"}},
		Containers: map[string]Container{},
	}

	user := cfg.GetUser("nonexistent")

	if user.Name != "default" {
		t.Errorf("expected default, got %s", user.Name)
	}
	if user.Password != "pass" {
		t.Errorf("expected pass, got %s", user.Password)
	}
}

func TestGetUser_PartialContainerConfig_PasswordFromDefaults(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{User: User{Name: "ignored", Password: "defaultpass"}},
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu", User: User{Name: "alice"}}, // No password
		},
	}

	user := cfg.GetUser("dev1")

	if user.Name != "alice" {
		t.Errorf("expected alice, got %s", user.Name)
	}
	if user.Password != "defaultpass" {
		t.Errorf("expected defaultpass, got %s", user.Password)
	}
}

func TestGetUser_PartialContainerConfig_PasswordHardcoded(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{User: User{Name: "ignored"}}, // No password in defaults
		Containers: map[string]Container{
			"dev1": {Image: "ubuntu", User: User{Name: "alice"}},
		},
	}

	user := cfg.GetUser("dev1")

	if user.Name != "alice" {
		t.Errorf("expected alice, got %s", user.Name)
	}
	if user.Password != "dev" {
		t.Errorf("expected dev (hardcoded), got %s", user.Password)
	}
}

func TestGetUser_DefaultsPartialConfig(t *testing.T) {
	cfg := &Config{
		Defaults:   Defaults{User: User{Name: "default"}}, // No password
		Containers: map[string]Container{"dev1": {Image: "ubuntu"}},
	}

	user := cfg.GetUser("dev1")

	if user.Name != "default" {
		t.Errorf("expected default, got %s", user.Name)
	}
	if user.Password != "dev" {
		t.Errorf("expected dev (hardcoded), got %s", user.Password)
	}
}

func TestLoad_WithUserConfig(t *testing.T) {
	withTempDir(t, func(dir string) {
		yaml := `project: test
defaults:
  user:
    name: devuser
    password: devpass
containers:
  dev1:
    image: ubuntu:24.04
  dev2:
    image: ubuntu:24.04
    user:
      name: customuser
      password: custompass
`
		if err := os.WriteFile(ConfigFile, []byte(yaml), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check defaults
		if cfg.Defaults.User.Name != "devuser" {
			t.Errorf("expected default user devuser, got %s", cfg.Defaults.User.Name)
		}
		if cfg.Defaults.User.Password != "devpass" {
			t.Errorf("expected default password devpass, got %s", cfg.Defaults.User.Password)
		}

		// Check dev1 has no user config
		if cfg.Containers["dev1"].User.Name != "" {
			t.Errorf("expected dev1 to have no user name, got %s", cfg.Containers["dev1"].User.Name)
		}

		// Check dev2 has custom user
		if cfg.Containers["dev2"].User.Name != "customuser" {
			t.Errorf("expected customuser, got %s", cfg.Containers["dev2"].User.Name)
		}
		if cfg.Containers["dev2"].User.Password != "custompass" {
			t.Errorf("expected custompass, got %s", cfg.Containers["dev2"].User.Password)
		}
	})
}

func TestSave_WithUserConfig(t *testing.T) {
	withTempDir(t, func(dir string) {
		cfg := &Config{
			Project:  "test",
			Defaults: Defaults{User: User{Name: "default", Password: "pass"}},
			Containers: map[string]Container{
				"dev1": {Image: "ubuntu", User: User{Name: "alice", Password: "secret"}},
			},
		}

		if err := cfg.Save(); err != nil {
			t.Fatalf("failed to save: %v", err)
		}

		loaded, err := Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		// Verify defaults preserved
		if loaded.Defaults.User.Name != "default" {
			t.Errorf("expected default user name, got %s", loaded.Defaults.User.Name)
		}
		if loaded.Defaults.User.Password != "pass" {
			t.Errorf("expected default password, got %s", loaded.Defaults.User.Password)
		}

		// Verify container user preserved
		if loaded.Containers["dev1"].User.Name != "alice" {
			t.Errorf("expected alice, got %s", loaded.Containers["dev1"].User.Name)
		}
		if loaded.Containers["dev1"].User.Password != "secret" {
			t.Errorf("expected secret, got %s", loaded.Containers["dev1"].User.Password)
		}
	})
}

func TestLoad_WithSnapshots(t *testing.T) {
	withTempDir(t, func(dir string) {
		yaml := `project: test
containers:
  dev1:
    image: ubuntu:24.04
    snapshots:
      initial-state:
        description: Initial state after setup
        created_at: "2024-01-15T10:30:00Z"
      checkpoint:
        description: Before refactoring
        created_at: "2024-01-15T14:00:00Z"
`
		if err := os.WriteFile(ConfigFile, []byte(yaml), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !cfg.HasSnapshot("dev1", "initial-state") {
			t.Error("expected initial-state snapshot")
		}
		if !cfg.HasSnapshot("dev1", "checkpoint") {
			t.Error("expected checkpoint snapshot")
		}
		if cfg.Containers["dev1"].Snapshots["checkpoint"].Description != "Before refactoring" {
			t.Error("unexpected description")
		}
	})
}
