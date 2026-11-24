// Package launchd installs the agent as a per-user LaunchAgent on macOS.
package launchd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ivanhreyes/isphub-print-agent/internal/config"
)

const label = "app.isphub.print-agent"

// BinaryPath is where install copies the executable.
func BinaryPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "isphub-print-agent"), nil
}

// PlistPath is the user LaunchAgent.
func PlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func logDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Logs", "ISPHubPrintAgent"), nil
}

// Plist is the launchd document for this binary.
func Plist(binary, workdir, logs string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, label, xmlText(binary), xmlText(workdir), xmlText(filepath.Join(logs, "agent.log")), xmlText(filepath.Join(logs, "agent.err.log")))
}

func xmlText(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
}

// Install copies the current executable and loads the LaunchAgent.
func Install(currentExe string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("la instalación automática solo está disponible en macOS")
	}
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	logs, err := logDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(logs, 0o755); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, "Library", "LaunchAgents"), 0o755); err != nil {
		return err
	}
	dest, err := BinaryPath()
	if err != nil {
		return err
	}
	if filepath.Clean(currentExe) != filepath.Clean(dest) {
		if err := copyFile(currentExe, dest); err != nil {
			return err
		}
	}
	plistPath, err := PlistPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(plistPath, []byte(Plist(dest, dir, logs)), 0o644); err != nil {
		return err
	}
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", os.Getuid()), plistPath).Run()
	if out, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %s (%s)", err, bytesTrim(out))
	}
	if out, err := exec.Command("launchctl", "kickstart", "-k", target).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl kickstart: %s (%s)", err, bytesTrim(out))
	}
	return nil
}

// Uninstall stops the agent and removes the LaunchAgent. The config file stays.
func Uninstall() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("la instalación automática solo está disponible en macOS")
	}
	plistPath, err := PlistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", os.Getuid()), plistPath).Run()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ServiceTarget is the launchctl name used by status.
func ServiceTarget() string {
	return fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func bytesTrim(raw []byte) string {
	text := string(raw)
	if len(text) > 300 {
		return text[:300]
	}
	return text
}
