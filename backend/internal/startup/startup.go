package startup

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
)

type Manager struct {
	Directory, Executable, Config string
	Run                           func(string) error
}

func Current(exe, config string) (Manager, error) {
	if runtime.GOOS != "windows" {
		return Manager{}, errors.New("startup shortcuts require Windows")
	}
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return Manager{}, errors.New("APPDATA not available")
	}
	return Manager{filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup"), exe, config, RunPowerShell}, nil
}
func quote(s string) string       { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func (m Manager) path() string    { return filepath.Join(m.Directory, "HomeClipboardCanvas.lnk") }
func (m Manager) Installed() bool { _, err := os.Stat(m.path()); return err == nil }
func (m Manager) Install() error {
	if !filepath.IsAbs(m.Executable) || !filepath.IsAbs(m.Config) || !filepath.IsAbs(m.Directory) {
		return errors.New("startup paths must be absolute")
	}
	if err := os.MkdirAll(m.Directory, 0700); err != nil {
		return err
	}
	return m.Run(shortcutScript(m.Executable, m.Config, m.path()))
}
func shortcutScript(exe, config, destination string) string {
	return "$ErrorActionPreference = 'Stop'\n$path = " + quote(destination) + "\n$shell = New-Object -ComObject WScript.Shell\n$link = $shell.CreateShortcut($path)\n$target = " + quote(exe) + "\n$arguments = " + quote(`serve --config "`+config+`"`) + "\nif (Test-Path -LiteralPath $path) {\n if ($link.TargetPath -ne $target -or $link.Arguments -ne $arguments) { throw 'An unrelated startup shortcut already exists' }\n exit 0\n}\n$link.TargetPath = $target\n$link.Arguments = $arguments\n$link.WorkingDirectory = " + quote(filepath.Dir(exe)) + "\n$link.Description = 'Home Clipboard Canvas HTTPS server'\n$link.WindowStyle = 7\n$link.Save()\n"
}
func (m Manager) Uninstall() error {
	if !m.Installed() {
		return nil
	}
	return m.Run("$ErrorActionPreference = 'Stop'\n$path = " + quote(m.path()) + "\n$link = (New-Object -ComObject WScript.Shell).CreateShortcut($path)\nif ($link.TargetPath -ne " + quote(m.Executable) + ") { throw 'Refusing to remove an unrelated shortcut' }\nRemove-Item -LiteralPath $path\n")
}
func RunPowerShell(script string) error {
	if runtime.GOOS != "windows" {
		return errors.New("PowerShell shortcut generation requires Windows")
	}
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(data[i*2:], unit)
	}
	command := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	if err := command.Run(); err != nil {
		return errors.New("startup shortcut operation failed")
	}
	return nil
}
