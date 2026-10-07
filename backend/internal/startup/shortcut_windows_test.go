package startup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsCreatesRealShortcutOnlyInTemporaryDirectory(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "日本語 O'Brien", "HomeClipboardCanvas.exe")
	config := filepath.Join(dir, "日本語 O'Brien", "config.json")
	m := Manager{Directory: dir, Executable: exe, Config: config, Run: RunPowerShell}
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(m.path())
	if err != nil || info.Size() < 76 {
		t.Fatal("valid Shell Link was not produced")
	}
	before, _ := os.ReadFile(m.path())
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(m.path())
	if string(before) != string(after) {
		t.Fatal("duplicate registration rewrote shortcut")
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if m.Installed() {
		t.Fatal("temporary shortcut not removed")
	}
}
