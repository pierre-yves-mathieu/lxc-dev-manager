package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMv_InvalidDestinationFormat(t *testing.T) {
	env := setupTestEnv(t)
	env.writeConfigWithContainer("dev1", "ubuntu:24.04")

	// Create a test file
	testFile := filepath.Join(env.dir, "testfile.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	// Missing colon in destination
	err := runMv(nil, []string{testFile, "dev1/home/dev/"})
	if err == nil {
		t.Fatal("expected error for invalid destination format")
	}
	if !strings.Contains(err.Error(), "invalid destination format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMv_EmptyDestinationPath(t *testing.T) {
	env := setupTestEnv(t)
	env.writeConfigWithContainer("dev1", "ubuntu:24.04")

	testFile := filepath.Join(env.dir, "testfile.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	err := runMv(nil, []string{testFile, "dev1:"})
	if err == nil {
		t.Fatal("expected error for empty destination path")
	}
	if !strings.Contains(err.Error(), "destination path cannot be empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMv_SourceNotExists(t *testing.T) {
	env := setupTestEnv(t)
	env.writeConfigWithContainer("dev1", "ubuntu:24.04")

	err := runMv(nil, []string{"/nonexistent/file.txt", "dev1:/home/dev/"})
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMv_ContainerNotInConfig(t *testing.T) {
	env := setupTestEnv(t)
	env.writeMinimalConfig()

	testFile := filepath.Join(env.dir, "testfile.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	err := runMv(nil, []string{testFile, "nonexistent:/home/dev/"})
	if err == nil {
		t.Fatal("expected error for container not in config")
	}
	if !strings.Contains(err.Error(), "not found in project config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMv_ContainerNotInLXC(t *testing.T) {
	env := setupTestEnv(t)
	env.writeConfigWithContainer("dev1", "ubuntu:24.04")
	env.setContainerNotExists("dev1")

	testFile := filepath.Join(env.dir, "testfile.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	err := runMv(nil, []string{testFile, "dev1:/home/dev/"})
	if err == nil {
		t.Fatal("expected error for container not in LXC")
	}
	if !strings.Contains(err.Error(), "does not exist in LXC") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMv_TildeExpansion(t *testing.T) {
	env := setupTestEnv(t)
	env.writeConfig(`project: ""
containers:
  dev1:
    image: ubuntu:24.04
defaults:
  user:
    name: myuser
`)
	env.setContainerExists("dev1", true)
	// Mock directory exists check
	env.mock.SetOutput("exec dev1 -- test -d /home/myuser/.ssh", "")
	// Mock file push
	env.mock.SetOutput("file push", "")
	// Mock chown
	env.mock.SetOutput("exec dev1 -- chown myuser:myuser /home/myuser/.ssh/key", "")

	testFile := filepath.Join(env.dir, "key")
	os.WriteFile(testFile, []byte("ssh key content"), 0644)

	err := runMv(nil, []string{testFile, "dev1:~/.ssh/key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the file push was called with expanded path
	foundPush := false
	for _, call := range env.mock.Calls {
		callStr := strings.Join(call.Args, " ")
		if strings.Contains(callStr, "file push") && strings.Contains(callStr, "/home/myuser/.ssh/key") {
			foundPush = true
			break
		}
	}
	if !foundPush {
		t.Errorf("expected file push with expanded path, got calls: %v", env.mock.Calls)
	}
}

func TestMv_SuccessfulFileCopy(t *testing.T) {
	env := setupTestEnv(t)
	env.writeConfigWithContainer("dev1", "ubuntu:24.04")
	env.setContainerExists("dev1", true)
	// Mock directory exists
	env.mock.SetOutput("exec dev1 -- test -d /home/dev", "")
	// Mock file push
	env.mock.SetOutput("file push", "")
	// Mock chown (default user is "dev")
	env.mock.SetOutput("exec dev1 -- chown dev:dev /home/dev/testfile.txt", "")

	testFile := filepath.Join(env.dir, "testfile.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	err := runMv(nil, []string{testFile, "dev1:/home/dev/testfile.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify chown was called
	foundChown := false
	for _, call := range env.mock.Calls {
		callStr := strings.Join(call.Args, " ")
		if strings.Contains(callStr, "chown") && strings.Contains(callStr, "dev:dev") {
			foundChown = true
			break
		}
	}
	if !foundChown {
		t.Errorf("expected chown call, got calls: %v", env.mock.Calls)
	}
}

func TestMv_DirectoryCopyWithRecursiveChown(t *testing.T) {
	env := setupTestEnv(t)
	env.writeConfigWithContainer("dev1", "ubuntu:24.04")
	env.setContainerExists("dev1", true)
	// Mock directory exists
	env.mock.SetOutput("exec dev1 -- test -d", "")
	// Mock file push with -r
	env.mock.SetOutput("file push -r", "")
	// Mock recursive chown
	env.mock.SetOutput("exec dev1 -- chown -R dev:dev", "")

	// Create a test directory
	testDir := filepath.Join(env.dir, "myproject")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "file1.txt"), []byte("content"), 0644)

	err := runMv(nil, []string{testDir, "dev1:/home/dev/myproject"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify recursive chown was called
	foundRecursiveChown := false
	for _, call := range env.mock.Calls {
		callStr := strings.Join(call.Args, " ")
		if strings.Contains(callStr, "chown -R") {
			foundRecursiveChown = true
			break
		}
	}
	if !foundRecursiveChown {
		t.Errorf("expected recursive chown call, got calls: %v", env.mock.Calls)
	}
}
