package agent

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ivanhreyes/isphub-print-agent/internal/config"
)

func TestHealthAndPrivateNetworkPreflight(t *testing.T) {
	svc := &Service{Config: config.Default(), Version: "0.1.0", Hostname: "mac"}
	req := httptest.NewRequest(http.MethodOptions, "/v1/print", nil)
	req.Header.Set("Origin", "https://demo.isphub.app")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatal(rec.Header())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://demo.isphub.app" {
		t.Fatal(rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestForeignOriginIsRejected(t *testing.T) {
	svc := &Service{Config: config.Default(), Version: "0.1.0"}
	req := httptest.NewRequest(http.MethodPost, "/v1/print", strings.NewReader(`{"kind":"test"}`))
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestSetupPageServed(t *testing.T) {
	svc := &Service{Config: config.Default(), Version: "0.1.0", Hostname: "mac"}
	for _, path := range []string{"/", "/setup"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status %d", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("%s content-type %q", path, ct)
		}
		if !strings.Contains(rec.Body.String(), "ISPHub") {
			t.Fatalf("%s no incluye la página del asistente", path)
		}
	}
}

func TestSetPrinterSaves(t *testing.T) {
	var saved config.File
	svc := &Service{
		Config:   config.Default(),
		Version:  "0.1.0",
		Hostname: "mac",
		Save:     func(c config.File) error { saved = c; return nil },
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/printer", strings.NewReader(`{"host":"192.168.0.20","port":9100,"paperWidthMm":80}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if saved.PrinterHost != "192.168.0.20" {
		t.Fatalf("host %q", saved.PrinterHost)
	}
	if saved.PrinterPort != 9100 {
		t.Fatalf("port %d", saved.PrinterPort)
	}
}

func TestSetPrinterRejectsPublicIP(t *testing.T) {
	svc := &Service{
		Config:   config.Default(),
		Version:  "0.1.0",
		Hostname: "mac",
		Save:     func(c config.File) error { return nil },
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/printer", strings.NewReader(`{"host":"8.8.8.8","port":9100,"paperWidthMm":80}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestDryRunPrint(t *testing.T) {
	cfg := config.Default()
	cfg.DryRun = true
	cfg.PrinterHost = "192.168.0.20"
	svc := &Service{Config: cfg, Version: "0.1.0", Hostname: "mostrador"}
	req := httptest.NewRequest(http.MethodPost, "/v1/print", strings.NewReader(`{"kind":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), `"dryRun":true`) {
		t.Fatal(string(body))
	}
}
