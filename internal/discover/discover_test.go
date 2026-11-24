package discover

import "testing"

func TestClassifyECAndBrother(t *testing.T) {
	ec := `<html><head><meta charset="gb2312"><title>Configures</title></head>`
	if classify(ec, "lwIP/1.3.1") != "ticket" {
		t.Fatal("EC page was not a ticket printer")
	}
	brother := `<html><title>Brother QL-810W</title></head>`
	if classify(brother, "debut/1.30") != "label" {
		t.Fatal("Brother was not a label printer")
	}
	if classify("<html><title>Router</title>", "nginx") != "unknown" {
		t.Fatal("router looked like a printer")
	}
}

func TestPrintableModel(t *testing.T) {
	if got := printableModel([]byte{0x12, 'E', 'C', '-', 'P', 'M', '-', '8', '0', '3', '3', '0', 0}); got != "EC-PM-80330" {
		t.Fatal(got)
	}
	if got := printableModel([]byte{0x16}); got != "" {
		t.Fatal(got)
	}
}
