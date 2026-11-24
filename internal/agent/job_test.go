package agent

import (
	"strings"
	"testing"
)

func TestPreparePaymentReceiptEnvelope(t *testing.T) {
	body := []byte(`{
	  "data": {
	    "payment": {"receiptNumber": "REC-1", "amountCents": 2500, "currency": "MXN", "method": "spei", "receivedAt": "2026-11-24T15:00:00Z"},
	    "customer": {"name": "Ana", "balanceCents": 0, "currency": "MXN"},
	    "invoices": [],
	    "summary": {"appliedCents": 2500, "creditCents": 0},
	    "print": {"count": 0},
	    "organization": {"name": "Norte"},
	    "config": {"paperWidthMm": 80, "title": "RECIBO DE PAGO", "showOrganizationName": true, "showPaymentMethod": true, "footerText": "Gracias."}
	  }
	}`)
	got, err := Prepare(body, 80, "mac")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"REC-1", "SPEI", "NORTE", "$25.00"} {
		if !strings.Contains(got.Preview, want) {
			t.Fatalf("missing %q\n%s", want, got.Preview)
		}
	}
}

func TestPrepareTestPage(t *testing.T) {
	got, err := Prepare([]byte(`{"kind":"test"}`), 80, "mostrador")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Data) < 20 {
		t.Fatal("empty test page")
	}
}

func TestPrepareBlocks(t *testing.T) {
	body := []byte(`{"kind":"blocks","document":{"format":"ISPHUB_PRINT_V1","paperWidthMm":80,"blocks":[{"type":"text","text":"Hola","align":"center"}]}}`)
	got, err := Prepare(body, 80, "mac")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Preview, "Hola") {
		t.Fatal(got.Preview)
	}
}
