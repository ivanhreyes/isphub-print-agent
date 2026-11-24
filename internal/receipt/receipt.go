// Package receipt turns the JSON of an ISPHub payment receipt into ESC/POS.
// The field names match PrintablePaymentReceipt in the ISPHub web app.
package receipt

import (
	"fmt"
	"strings"
	"time"

	"github.com/ivanhreyes/isphub-print-agent/internal/escpos"
)

const appDownloadURL = "https://isphub.app/app"

// PaymentReceipt is the `data` object of GET /billing/payments/:id/print-receipt.
type PaymentReceipt struct {
	Payment struct {
		ID            string `json:"id"`
		Folio         string `json:"folio"`
		ReceiptNumber string `json:"receiptNumber"`
		AmountCents   int64  `json:"amountCents"`
		Currency      string `json:"currency"`
		Method        string `json:"method"`
		ReceivedAt    string `json:"receivedAt"`
	} `json:"payment"`
	Customer struct {
		Name         string `json:"name"`
		Phone        string `json:"phone"`
		BalanceCents int64  `json:"balanceCents"`
		Currency     string `json:"currency"`
	} `json:"customer"`
	Invoices []Invoice `json:"invoices"`
	Summary  struct {
		AppliedCents int64 `json:"appliedCents"`
		CreditCents  int64 `json:"creditCents"`
	} `json:"summary"`
	Print struct {
		Count         int    `json:"count"`
		LastPrintedAt string `json:"lastPrintedAt"`
	} `json:"print"`
	Organization struct {
		Name    string `json:"name"`
		Logo    string `json:"logo"`
		RFC     string `json:"rfc"`
		Address string `json:"address"`
		Phone   string `json:"phone"`
		Email   string `json:"email"`
	} `json:"organization"`
	Config Config `json:"config"`
}

// Invoice is one application of the payment.
type Invoice struct {
	ID           string `json:"id"`
	Folio        string `json:"folio"`
	AppliedCents int64  `json:"appliedCents"`
	Currency     string `json:"currency"`
}

// Config mirrors paymentReceiptConfigSchema. Logo raster is not printed in
// this version; the organization name still is, when the flag asks for it.
type Config struct {
	PaperWidthMm         int           `json:"paperWidthMm"`
	Title                string        `json:"title"`
	HeaderText           string        `json:"headerText"`
	FooterText           string        `json:"footerText"`
	ShowLogo             bool          `json:"showLogo"`
	ShowOrganizationName bool          `json:"showOrganizationName"`
	ShowRFC              bool          `json:"showRfc"`
	ShowAddress          bool          `json:"showAddress"`
	ShowPhone            bool          `json:"showPhone"`
	ShowEmail            bool          `json:"showEmail"`
	ShowCustomerPhone    bool          `json:"showCustomerPhone"`
	ShowInvoiceReference bool          `json:"showInvoiceReference"`
	ShowPaymentMethod    bool          `json:"showPaymentMethod"`
	ShowRemainingBalance bool          `json:"showRemainingBalance"`
	ShowInvoiceBreakdown bool          `json:"showInvoiceBreakdown"`
	ShowAppDownloadQr    bool          `json:"showAppDownloadQr"`
	CustomFields         []CustomField `json:"customFields"`
}

// CustomField is an extra line chosen in ISPHub settings.
type CustomField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// PaperWidth returns 58 or 80. The EC-PM-80330 is an 80 mm printer; a missing
// width stays on 80 so a test ticket is not squeezed to 32 columns.
func PaperWidth(cfg Config) int {
	if cfg.PaperWidthMm == 58 {
		return 58
	}
	return 80
}

// Blocks lays the receipt out in the same order as the ISPHub web preview.
func Blocks(r PaymentReceipt) ([]escpos.Block, error) {
	if strings.TrimSpace(r.Payment.ReceiptNumber) == "" {
		return nil, fmt.Errorf("el recibo no trae folio")
	}
	cfg := r.Config
	var blocks []escpos.Block
	text := func(value, align, size string, bold bool) {
		if strings.TrimSpace(value) == "" {
			return
		}
		blocks = append(blocks, escpos.Block{Type: "text", Text: value, Align: align, Size: size, Bold: bold})
	}
	pair := func(left, right string) {
		blocks = append(blocks, escpos.Block{Type: "pair", Left: left, Right: right})
	}
	sep := func() {
		blocks = append(blocks, escpos.Block{Type: "separator"})
	}

	if cfg.ShowOrganizationName {
		text(strings.ToUpper(r.Organization.Name), "center", "large", true)
	}
	if cfg.ShowRFC && strings.TrimSpace(r.Organization.RFC) != "" {
		text("RFC: "+strings.TrimSpace(r.Organization.RFC), "center", "normal", false)
	}
	if cfg.ShowAddress {
		text(r.Organization.Address, "center", "normal", false)
	}
	if cfg.ShowPhone && strings.TrimSpace(r.Organization.Phone) != "" {
		text("Tel. "+strings.TrimSpace(r.Organization.Phone), "center", "normal", false)
	}
	if cfg.ShowEmail {
		text(r.Organization.Email, "center", "normal", false)
	}
	text(cfg.HeaderText, "center", "normal", false)

	sep()
	title := strings.TrimSpace(cfg.Title)
	if title == "" {
		title = "RECIBO DE PAGO"
	}
	text(title, "center", "normal", true)
	if r.Print.Count > 0 {
		text(fmt.Sprintf("COPIA / REIMPRESIÓN · %d previa(s)", r.Print.Count), "center", "normal", true)
	} else {
		text("ORIGINAL", "center", "normal", true)
	}

	pair("Folio", r.Payment.ReceiptNumber)
	pair("Fecha", formatWhen(r.Payment.ReceivedAt))
	pair("Cliente", blank(r.Customer.Name, "Sin nombre"))
	if cfg.ShowCustomerPhone && strings.TrimSpace(r.Customer.Phone) != "" {
		pair("Teléfono", strings.TrimSpace(r.Customer.Phone))
	}
	if cfg.ShowInvoiceReference && len(r.Invoices) == 1 {
		pair("Factura", r.Invoices[0].Folio)
	}
	if cfg.ShowPaymentMethod {
		pair("Forma", methodLabel(r.Payment.Method))
	}

	if cfg.ShowInvoiceBreakdown && len(r.Invoices) > 0 {
		sep()
		text("APLICACIÓN DEL PAGO", "left", "normal", true)
		for _, invoice := range r.Invoices {
			pair("Factura "+invoice.Folio, money(invoice.AppliedCents, invoice.Currency))
		}
		if r.Summary.CreditCents > 0 {
			pair("Saldo a favor", money(r.Summary.CreditCents, r.Payment.Currency))
		}
	}

	sep()
	pair("TOTAL PAGADO", money(r.Payment.AmountCents, r.Payment.Currency))
	if cfg.ShowRemainingBalance {
		currency := r.Customer.Currency
		if currency == "" {
			currency = r.Payment.Currency
		}
		text("Saldo de la cuenta: "+money(r.Customer.BalanceCents, currency), "right", "normal", false)
	}

	if len(cfg.CustomFields) > 0 {
		sep()
		for _, field := range cfg.CustomFields {
			if strings.TrimSpace(field.Label) == "" {
				continue
			}
			pair(field.Label, field.Value)
		}
	}

	if cfg.ShowAppDownloadQr {
		sep()
		text("DESCARGA LA APP DE CLIENTES", "center", "normal", true)
		text("Consulta saldo, pagos, facturas y soporte.", "center", "normal", false)
		blocks = append(blocks, escpos.Block{Type: "qr", Value: appDownloadURL})
		text("isphub.app/app", "center", "normal", false)
	}

	sep()
	text(cfg.FooterText, "center", "normal", false)
	text("Generado por ISP Hub", "center", "normal", false)
	return blocks, nil
}

// Encode builds the bytes to send to the printer.
func Encode(r PaymentReceipt) ([]byte, error) {
	blocks, err := Blocks(r)
	if err != nil {
		return nil, err
	}
	return escpos.Build(PaperWidth(r.Config), blocks, true)
}

// Preview is the dry-run text of a receipt.
func Preview(r PaymentReceipt) (string, error) {
	blocks, err := Blocks(r)
	if err != nil {
		return "", err
	}
	return escpos.Preview(PaperWidth(r.Config), blocks), nil
}

func formatWhen(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.In(time.Local).Format("02/01/2006 15:04")
		}
	}
	return value
}

func methodLabel(method string) string {
	labels := map[string]string{
		"cash":           "Efectivo",
		"bank_transfer":  "Transferencia",
		"card":           "Tarjeta",
		"gateway":        "Pago en línea",
		"oxxo":           "OXXO",
		"spei":           "SPEI",
		"pix":            "PIX",
		"credit_balance": "Saldo a favor",
	}
	if label, ok := labels[method]; ok {
		return label
	}
	if strings.TrimSpace(method) == "" {
		return "No indicada"
	}
	return method
}

func money(cents int64, currency string) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	whole := cents / 100
	frac := cents % 100
	number := thousands(whole) + fmt.Sprintf(".%02d", frac)
	code := strings.ToUpper(strings.TrimSpace(currency))
	if code == "" || code == "MXN" {
		return sign + "$" + number
	}
	return sign + number + " " + code
}

func thousands(n int64) string {
	raw := fmt.Sprintf("%d", n)
	if len(raw) <= 3 {
		return raw
	}
	lead := len(raw) % 3
	if lead == 0 {
		lead = 3
	}
	var b strings.Builder
	b.WriteString(raw[:lead])
	for i := lead; i < len(raw); i += 3 {
		b.WriteByte(',')
		b.WriteString(raw[i : i+3])
	}
	return b.String()
}

func blank(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
