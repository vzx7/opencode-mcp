package mcp

import (
	"testing"
)

func TestNewServer(t *testing.T) {
	cfg := Config{
		Provider: "mock",
	}

	server := NewServer(cfg)

	if server == nil {
		t.Error("expected server to be created")
	}
}

func TestNewServer_NoDebugDir(t *testing.T) {
	cfg := Config{
		Provider: "mock",
	}

	server := NewServer(cfg)

	if server == nil {
		t.Error("expected server to be created")
	}
}