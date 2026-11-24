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
