package main

import (
	"testing"
	"time"
)

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("TEST_KEY", "from_env")
	if got := envOrDefault("TEST_KEY", "fallback"); got != "from_env" {
		t.Errorf("envOrDefault() = %q, want %q", got, "from_env")
	}
	if got := envOrDefault("NONEXISTENT_KEY", "fallback"); got != "fallback" {
		t.Errorf("envOrDefault() = %q, want %q", got, "fallback")
	}
}

func TestEnvOrDefaultInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if got := envOrDefaultInt("TEST_INT", 1); got != 42 {
		t.Errorf("envOrDefaultInt() = %d, want 42", got)
	}
	if got := envOrDefaultInt("NONEXISTENT", 1); got != 1 {
		t.Errorf("envOrDefaultInt() = %d, want 1", got)
	}
	// Invalid value falls back to default
	t.Setenv("TEST_INT_BAD", "notanumber")
	if got := envOrDefaultInt("TEST_INT_BAD", 5); got != 5 {
		t.Errorf("envOrDefaultInt() = %d, want 5 for invalid input", got)
	}
}

func TestEnvOrDefaultDuration(t *testing.T) {
	t.Setenv("TEST_DUR", "30s")
	if got := envOrDefaultDuration("TEST_DUR", 5*time.Second); got != 30*time.Second {
		t.Errorf("envOrDefaultDuration() = %v, want 30s", got)
	}
	if got := envOrDefaultDuration("NONEXISTENT", 5*time.Second); got != 5*time.Second {
		t.Errorf("envOrDefaultDuration() = %v, want 5s", got)
	}
}
