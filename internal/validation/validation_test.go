package validation

import (
	"strings"
	"testing"
)

func TestValidateContainerName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		errMsg  string
	}{
		// Valid names
		{"dev", false, ""},
		{"dev1", false, ""},
		{"my-container", false, ""},
		{"a", false, ""},
		{"MyContainer", false, ""},
		{"a1b2c3", false, ""},

		// Invalid: empty
		{"", true, "cannot be empty"},

		// Invalid: starts with number
		{"1dev", true, "must start with a letter"},
		{"123", true, "must start with a letter"},

		// Invalid: spaces
		{"my container", true, "cannot contain spaces"},

		// Invalid: special characters
		{"my_container", true, "cannot contain underscores"},
		{"my.container", true, "invalid characters"},
		{"my@container", true, "invalid characters"},

		// Invalid: hyphens at edges
		{"-dev", true, "invalid characters"},
		{"dev-", true, "cannot start or end with a hyphen"},

		// Invalid: consecutive hyphens
		{"dev--test", true, "consecutive hyphens"},

		// Invalid: reserved
		{"list", true, "reserved name"},
		{"delete", true, "reserved name"},
		{"create", true, "reserved name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContainerName(%q) error = %v, wantErr %v",
					tt.name, err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %v", tt.errMsg, err)
				}
			}
		})
	}
}

func TestValidateContainerName_TooLong(t *testing.T) {
	// Create a name that's exactly at the limit
	maxName := "a" + strings.Repeat("b", MaxContainerNameLength-1)
	if err := ValidateContainerName(maxName); err != nil {
		t.Errorf("name at max length should be valid: %v", err)
	}

	// Create a name that's one over the limit
	tooLong := maxName + "c"
	err := ValidateContainerName(tooLong)
	if err == nil {
		t.Error("expected error for name over max length")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("expected 'too long' error, got: %v", err)
	}
}

func TestValidateFullContainerName(t *testing.T) {
	tests := []struct {
		project   string
		container string
		wantErr   bool
		errMsg    string
	}{
		// Valid combinations
		{"", "dev", false, ""},
		{"myproject", "dev", false, ""},
		{"project", "container", false, ""},

		// Invalid container name
		{"project", "1dev", true, "must start with a letter"},
		{"project", "", true, "cannot be empty"},

		// Combined length too long
		{strings.Repeat("a", 30), strings.Repeat("b", 35), true, "too long"},
	}

	for _, tt := range tests {
		name := tt.project + "/" + tt.container
		t.Run(name, func(t *testing.T) {
			err := ValidateFullContainerName(tt.project, tt.container)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFullContainerName(%q, %q) error = %v, wantErr %v",
					tt.project, tt.container, err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %v", tt.errMsg, err)
				}
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		port    int
		wantErr bool
	}{
		{80, false},
		{8080, false},
		{65535, false},
		{1, false},
		{0, true},
		{-1, true},
		{65536, true},
		{99999, true},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.port)), func(t *testing.T) {
			err := ValidatePort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%d) error = %v, wantErr %v",
					tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePorts(t *testing.T) {
	tests := []struct {
		name    string
		ports   []int
		wantErr bool
		errMsg  string
	}{
		{"empty", []int{}, false, ""},
		{"single valid", []int{8080}, false, ""},
		{"multiple valid", []int{3000, 8000, 5432}, false, ""},
		{"invalid port", []int{8080, 99999}, true, "invalid port"},
		{"duplicate", []int{8080, 3000, 8080}, true, "duplicate"},
		{"zero", []int{0}, true, "invalid port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePorts(tt.ports)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePorts(%v) error = %v, wantErr %v",
					tt.ports, err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %v", tt.errMsg, err)
				}
			}
		})
	}
}
