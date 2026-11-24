// Package config stores the agent settings for one Mac user.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ivanhreyes/isphub-print-agent/internal/netaddr"
)

const dirName = "ISPHubPrintAgent"

// File is the on-disk configuration. The agent secret stays in this file
// with mode 0600 and is never printed by the status command.
type File struct {
	PrinterHost     string   `json:"printerHost"`
	PrinterPort     int      `json:"printerPort"`
	PaperWidthMm    int      `json:"paperWidthMm"`
	Listen          string   `json:"listen"`
	ServerURL       string   `json:"serverURL"`
	EnrollmentToken string   `json:"enrollmentToken,omitempty"`
	AgentID         string   `json:"agentId,omitempty"`
	AgentSecret     string   `json:"agentSecret,omitempty"`
	AllowedOrigins  []string `json:"allowedOrigins,omitempty"`
	DryRun          bool     `json:"dryRun"`
}

// Default is a usable file before the cashier picks a printer.
func Default() File {
	return File{
		PrinterPort:  9100,
		PaperWidthMm: 80,
		Listen:       "127.0.0.1:8787",
	}
}

// Dir is ~/Library/Application Support/ISPHubPrintAgent.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, dirName), nil
}

// Path is the config.json location.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the file or returns Default when it does not exist yet.
func Load() (File, error) {
	path, err := Path()
	if err != nil {
		return File{}, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return File{}, err
	}
	cfg := Default()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return File{}, fmt.Errorf("la configuración no es JSON válido: %w", err)
	}
	cfg.normalize()
	return cfg, cfg.Validate()
}

// Save writes the file with permissions limited to the current user.
func Save(cfg File) error {
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	path := filepath.Join(dir, "config.json")
	return os.WriteFile(path, raw, 0o600)
}

func (cfg *File) normalize() {
	cfg.PrinterHost = strings.TrimSpace(cfg.PrinterHost)
	cfg.ServerURL = strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.EnrollmentToken = strings.TrimSpace(cfg.EnrollmentToken)
	cfg.AgentID = strings.TrimSpace(cfg.AgentID)
	cfg.AgentSecret = strings.TrimSpace(cfg.AgentSecret)
	if cfg.PrinterPort == 0 {
		cfg.PrinterPort = 9100
	}
	if cfg.PaperWidthMm != 58 {
		cfg.PaperWidthMm = 80
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8787"
	}
}

// Validate checks the addresses before the agent opens a socket.
func (cfg File) Validate() error {
	if cfg.PrinterHost != "" && !netaddr.PrivateIPv4(cfg.PrinterHost) {
		return fmt.Errorf("la IP de la impresora tiene que ser privada")
	}
	if _, err := netaddr.Port(cfg.PrinterPort); err != nil {
		return err
	}
	if cfg.PaperWidthMm != 58 && cfg.PaperWidthMm != 80 {
		return fmt.Errorf("el papel tiene que ser 58 o 80 mm")
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return fmt.Errorf("la dirección local %q no es válida", cfg.Listen)
	}
	if host != "127.0.0.1" && host != "localhost" {
		return fmt.Errorf("el agente solo puede escuchar en 127.0.0.1")
	}
	if cfg.ServerURL == "" {
		return nil
	}
	parsed, err := url.Parse(cfg.ServerURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("la URL de ISPHub no es válida")
	}
	loopback := parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost"
	if parsed.Scheme == "https" || (parsed.Scheme == "http" && loopback) {
		return nil
	}
	return fmt.Errorf("la URL de ISPHub tiene que ser https")
}

// Redacted hides the enrollment token and the agent secret.
func (cfg File) Redacted() File {
	out := cfg
	if out.EnrollmentToken != "" {
		out.EnrollmentToken = "••••"
	}
	if out.AgentSecret != "" {
		out.AgentSecret = "••••"
	}
	return out
}
