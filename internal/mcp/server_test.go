package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServer_DebugDirCreation(t *testing.T) {
	tmpDir := t.TempDir()
	debugDir := filepath.Join(tmpDir, "debug")

	cfg := Config{
		ProjectPath: tmpDir,
		DebugDir:    debugDir,
		Provider:   "mock",
	}

	server := NewServer(cfg)

	if server.debugDir != debugDir {
		t.Errorf("expected debugDir %s, got %s", debugDir, server.debugDir)
	}

	if _, err := os.Stat(debugDir); os.IsNotExist(err) {
		t.Error("debug directory was not created")
	}
}

func TestNewServer_DefaultDebugDir(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		ProjectPath: tmpDir,
		Provider:  "mock",
	}

	server := NewServer(cfg)

	expectedDebugDir := filepath.Join(tmpDir, "debug")
	if server.debugDir != expectedDebugDir {
		t.Errorf("expected debugDir %s, got %s", expectedDebugDir, server.debugDir)
	}

	if _, err := os.Stat(expectedDebugDir); os.IsNotExist(err) {
		t.Error("default debug directory was not created")
	}
}

func TestNewServer_NoDebugDir(t *testing.T) {
	cfg := Config{
		Provider: "mock",
	}

	server := NewServer(cfg)

	if server.debugDir != "" {
		t.Errorf("expected empty debugDir, got %s", server.debugDir)
	}
}