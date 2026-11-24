package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ivanhreyes/isphub-print-agent/internal/config"
	"github.com/ivanhreyes/isphub-print-agent/internal/discover"
	"github.com/ivanhreyes/isphub-print-agent/internal/printer"
)

// Service is the loopback API the ISPHub page calls, plus the optional
// outbound poller once ISPHub exposes the print queue.
type Service struct {
	mu       sync.Mutex
	Config   config.File
	Version  string
	Hostname string
	Save     func(config.File) error
	Send     func(ctx context.Context, host string, port int, data []byte) error
}

// Handler is the HTTP API bound to 127.0.0.1.
func (s *Service) Handler() http.Handler {
	if s.Send == nil {
		s.Send = printer.Send
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("GET /v1/printers", s.printers)
	mux.HandleFunc("POST /v1/print", s.print)
	mux.HandleFunc("POST /v1/printer", s.setPrinter)
	mux.HandleFunc("OPTIONS /v1/health", s.preflight)
	mux.HandleFunc("OPTIONS /v1/printers", s.preflight)
	mux.HandleFunc("OPTIONS /v1/print", s.preflight)
	mux.HandleFunc("OPTIONS /v1/printer", s.preflight)
	mux.HandleFunc("GET /{$}", s.setup)
	mux.HandleFunc("GET /setup", s.setup)
	return mux
}

// Listen starts the loopback server and stops it when ctx is canceled.
func (s *Service) Listen(ctx context.Context) error {
	server := &http.Server{
		Addr:              s.Config.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp4", s.Config.Listen)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Service) health(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r) {
		return
	}
	s.mu.Lock()
	cfg := s.Config
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": s.Version,
		"printer": map[string]any{
			"host":         cfg.PrinterHost,
			"port":         cfg.PrinterPort,
			"paperWidthMm": cfg.PaperWidthMm,
		},
		"dryRun": cfg.DryRun,
	})
}

func (s *Service) printers(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	found, err := discover.Scan(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"printers": found})
}

// setup serves the single-file configuration page.
func (s *Service) setup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(setupHTML)
}

// setPrinter persists the printer address chosen in the setup page. It only
// touches host, port and paper width; the enrollment token and agent secret
// stay untouched.
func (s *Service) setPrinter(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r) {
		return
	}
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": "el cuerpo tiene que ser JSON"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no se pudo leer la petición"})
		return
	}
	var req struct {
		Host         string `json:"host"`
		Port         int    `json:"port"`
		PaperWidthMm int    `json:"paperWidthMm"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "el cuerpo no es JSON válido"})
		return
	}
	req.Host = strings.TrimSpace(req.Host)
	if req.Host != "" && req.Port == 0 {
		req.Port = 9100
	}
	if req.PaperWidthMm == 0 {
		req.PaperWidthMm = 80
	}
	s.mu.Lock()
	cfg := s.Config
	s.mu.Unlock()
	if req.Host != "" {
		cfg.PrinterHost = req.Host
	}
	if req.Port > 0 {
		cfg.PrinterPort = req.Port
	}
	if req.PaperWidthMm == 58 || req.PaperWidthMm == 80 {
		cfg.PaperWidthMm = req.PaperWidthMm
	}
	if err := cfg.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if s.Save == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no hay almacenamiento de configuración"})
		return
	}
	if err := s.Save(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.mu.Lock()
	s.Config = cfg
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"printer": map[string]any{
			"host":         cfg.PrinterHost,
			"port":         cfg.PrinterPort,
			"paperWidthMm": cfg.PaperWidthMm,
		},
	})
}

func (s *Service) print(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r) {
		return
	}
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": "el cuerpo tiene que ser JSON"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no se pudo leer el trabajo"})
		return
	}
	s.mu.Lock()
	cfg := s.Config
	s.mu.Unlock()
	prepared, err := Prepare(body, cfg.PaperWidthMm, s.Hostname)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	host, port := cfg.PrinterHost, cfg.PrinterPort
	var override struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	_ = json.Unmarshal(body, &override)
	if strings.TrimSpace(override.Host) != "" {
		host = strings.TrimSpace(override.Host)
	}
	if override.Port > 0 {
		port = override.Port
	}
	if host == "" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "configura la IP de la impresora antes de imprimir"})
		return
	}
	if cfg.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dryRun": true, "bytes": len(prepared.Data), "preview": prepared.Preview})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.Send(ctx, host, port, prepared.Data); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(prepared.Data)})
}

func (s *Service) preflight(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) allow(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if !originAllowed(origin, s.Config.AllowedOrigins) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "este sitio no puede usar el agente"})
		return false
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Vary", "Origin")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type")
	h.Set("Access-Control-Allow-Private-Network", "true")
	return true
}

func originAllowed(origin string, extra []string) bool {
	parsed, err := http.NewRequest(http.MethodGet, origin, nil)
	if err != nil {
		return false
	}
	// NewRequest accepts a URL; reuse url.Parse via the request URL.
	host := parsed.URL.Hostname()
	scheme := parsed.URL.Scheme
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return scheme == "http" || scheme == "https"
	}
	if scheme == "https" && (host == "isphub.app" || strings.HasSuffix(host, ".isphub.app")) {
		return true
	}
	for _, item := range extra {
		if strings.EqualFold(strings.TrimSpace(item), origin) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
