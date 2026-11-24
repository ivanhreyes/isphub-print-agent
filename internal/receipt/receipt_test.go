package receipt

import (
	"strings"
	"testing"
	"time"
)

func sample() PaymentReceipt {
	var r PaymentReceipt
	r.Payment.ReceiptNumber = "REC-20261124-ABCDEF123456"
	r.Payment.AmountCents = 123456
	r.Payment.Currency = "MXN"
	r.Payment.Method = "cash"
	r.Payment.ReceivedAt = "2026-11-24T18:30:00Z"
	r.Customer.Name = "María Núñez"
	r.Customer.Phone = "5555550100"
	r.Customer.BalanceCents = 5000
	r.Customer.Currency = "MXN"
	r.Invoices = []Invoice{{Folio: "A-10", AppliedCents: 100000, Currency: "MXN"}}
	r.Summary.CreditCents = 23456
	r.Organization.Name = "Red Norte"
	r.Organization.RFC = "RNO010101AAA"
	r.Organization.Address = "Calle 1"
	r.Organization.Phone = "5550001111"
	r.Config = Config{
		PaperWidthMm:         80,
		Title:                "RECIBO DE PAGO",
		HeaderText:           "Sucursal centro",
		FooterText:           "Gracias por su pago.",
		ShowOrganizationName: true,
		ShowRFC:              true,
		ShowAddress:          true,
		ShowPhone:            true,
		ShowCustomerPhone:    true,
		ShowInvoiceReference: true,
		ShowPaymentMethod:    true,
		ShowRemainingBalance: true,
		ShowInvoiceBreakdown: true,
		ShowAppDownloadQr:    true,
		CustomFields:         []CustomField{{Label: "Caja", Value: "1"}},
	}
	return r
}

func TestPreviewMatchesWebOrder(t *testing.T) {
	r := sample()
	text, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 24, 18, 30, 0, 0, time.UTC).In(time.Local).Format("02/01/2006 15:04")
	for _, want := range []string{
		"RED NORTE",
		"RFC: RNO010101AAA",
		"Sucursal centro",
		"ORIGINAL",
		"REC-20261124-ABCDEF123456",
		when,
		"María Núñez",
		"Efectivo",
		"Factura A-10",
		"$1,000.00",
		"Saldo a favor",
		"$234.56",
		"TOTAL PAGADO",
		"$1,234.56",
		"Saldo de la cuenta: $50.00",
		"Caja",
		"https://isphub.app/app",
		"Gracias por su pago.",
		"Generado por ISP Hub",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "COPIA") {
		t.Fatal("original was marked as a copy")
	}
}

func TestReprintBanner(t *testing.T) {
	r := sample()
	r.Print.Count = 2
	text, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "COPIA / REIMPRESIÓN · 2 previa(s)") {
		t.Fatal(text)
	}
}

func TestHiddenFlagsStayOffTheTicket(t *testing.T) {
	r := sample()
	r.Config.ShowRFC = false
	r.Config.ShowAppDownloadQr = false
	r.Config.ShowCustomerPhone = false
	text, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "RNO010101AAA") || strings.Contains(text, "isphub.app") || strings.Contains(text, "5555550100") {
		t.Fatal(text)
	}
}

func TestRequiresFolio(t *testing.T) {
	r := sample()
	r.Payment.ReceiptNumber = " "
	if _, err := Encode(r); err == nil {
		t.Fatal("expected folio error")
	}
}

func TestEncodeFits80mm(t *testing.T) {
	data, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 {
		t.Fatalf("ticket too short: %d", len(data))
	}
}
