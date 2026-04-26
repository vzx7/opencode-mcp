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

func TestSaveDebugResponse(t *testing.T) {
	tmpDir := t.TempDir()
	debugDir := filepath.Join(tmpDir, "debug")
	os.MkdirAll(debugDir, 0755)

	cfg := Config{
		DebugDir: debugDir,
		Provider: "mock",
	}

	server := NewServer(cfg)
	server.saveDebugResponse("module_audit", "# Test Report\n\nContent here")

	files, err := os.ReadDir(debugDir)
	if err != nil {
		t.Fatalf("failed to read debug dir: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}

	content, err := os.ReadFile(filepath.Join(debugDir, files[0].Name()))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expectedContent := "# MODULE_AUDIT\n\n**Time:**"
	if string(content[:len(expectedContent)]) != expectedContent {
		t.Errorf("unexpected file header: got %s", string(content[:50]))
	}
}

func TestSaveDebugResponse_EmptyDebugDir(t *testing.T) {
	cfg := Config{
		Provider: "mock",
	}

	server := NewServer(cfg)
	server.saveDebugResponse("module_audit", "# Test")
}

func TestSaveDebugResponse_EmptyContent(t *testing.T) {
	tmpDir := t.TempDir()
	debugDir := filepath.Join(tmpDir, "debug")
	os.MkdirAll(debugDir, 0755)

	cfg := Config{
		DebugDir: debugDir,
		Provider: "mock",
	}

	server := NewServer(cfg)
	server.saveDebugResponse("module_audit", "")

	files, err := os.ReadDir(debugDir)
	if err != nil {
		t.Fatalf("failed to read debug dir: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}