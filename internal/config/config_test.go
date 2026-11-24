package config

import "testing"

func TestRejectsPublicPrinterAndOpenListen(t *testing.T) {
	cfg := Default()
	cfg.PrinterHost = "8.8.8.8"
	if err := cfg.Validate(); err == nil {
		t.Fatal("public printer accepted")
	}
	cfg = Default()
	cfg.Listen = "0.0.0.0:8787"
	if err := cfg.Validate(); err == nil {
		t.Fatal("lan listen accepted")
	}
	cfg = Default()
	cfg.ServerURL = "http://isphub.app"
	if err := cfg.Validate(); err == nil {
		t.Fatal("plain http cloud url accepted")
	}
}

func TestLocalDevServerIsAllowed(t *testing.T) {
	cfg := Default()
	cfg.ServerURL = "http://127.0.0.1:3000"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
