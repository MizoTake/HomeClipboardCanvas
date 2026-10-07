package startup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShortcutScriptQuotesPathsAndUsesExplicitConfig(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "O'Brien", "HomeClipboardCanvas.exe")
	config := filepath.Join(dir, "O'Brien", "config.json")
	script := shortcutScript(exe, config, filepath.Join(dir, "HomeClipboardCanvas.lnk"))
	if !strings.Contains(script, "O''Brien") || !strings.Contains(script, "--config") || !strings.Contains(script, "CreateShortcut") || !strings.Contains(script, "Test-Path -LiteralPath") {
		t.Fatal("shortcut script is incomplete or unsafe")
	}
}

func TestStartupInstallationUsesInjectedDirectoryAndRunner(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "HomeClipboardCanvas.exe")
	config := filepath.Join(dir, "config.json")
	manager := Manager{Directory: dir, Executable: exe, Config: config, Run: func(script string) error {
		if !strings.Contains(script, dir) {
			t.Fatal("shortcut escaped temporary directory")
		}
		return os.WriteFile(filepath.Join(dir, "HomeClipboardCanvas.lnk"), []byte("test shortcut"), 0600)
	}}
	if err := manager.Install(); err != nil {
		t.Fatal(err)
	}
	if !manager.Installed() {
		t.Fatal("shortcut not created")
	}
	manager.Run = func(script string) error {
		if !strings.Contains(script, "TargetPath") {
			t.Fatal("uninstall does not check owner")
		}
		return os.Remove(filepath.Join(dir, "HomeClipboardCanvas.lnk"))
	}
	if err := manager.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if manager.Installed() {
		t.Fatal("shortcut not removed")
	}
}
