package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ivanhreyes/isphub-print-agent/internal/escpos"
	"github.com/ivanhreyes/isphub-print-agent/internal/receipt"
)

// Prepared is one ticket ready for the thermal printer.
type Prepared struct {
	Data    []byte
	Preview string
	Paper   int
}

// Prepare turns an ISPHub print body into ESC/POS. The body may be the receipt
// itself, `{ "data": <receipt> }`, `{ "kind": "test" }`, or an ISPHUB_PRINT_V1
// document.
func Prepare(body []byte, fallbackPaper int, hostname string) (Prepared, error) {
	if fallbackPaper != 58 {
		fallbackPaper = 80
	}
	body = bytesTrim(body)
	if len(body) == 0 {
		return Prepared{}, fmt.Errorf("el trabajo de impresión está vacío")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return Prepared{}, fmt.Errorf("el trabajo no es JSON válido")
	}
	kind := jsonString(probe["kind"])
	switch kind {
	case "test":
		data, err := receipt.TestPage(hostname, paperFrom(probe, fallbackPaper))
		if err != nil {
			return Prepared{}, err
		}
		return Prepared{Data: data, Paper: paperFrom(probe, fallbackPaper), Preview: "página de prueba"}, nil
	case "blocks":
		return prepareBlocks(probe, fallbackPaper)
	case "payment-receipt", "":
		payload := body
		if kind == "payment-receipt" {
			if raw, ok := probe["receipt"]; ok {
				payload = raw
			}
		}
		if raw, ok := probe["data"]; ok && !hasPayment(probe) {
			payload = raw
		}
		return prepareReceipt(payload, fallbackPaper)
	default:
		return Prepared{}, fmt.Errorf("tipo de impresión %q no está soportado", kind)
	}
}

func prepareReceipt(payload []byte, fallbackPaper int) (Prepared, error) {
	var ticket receipt.PaymentReceipt
	if err := json.Unmarshal(payload, &ticket); err != nil {
		return Prepared{}, fmt.Errorf("el recibo de ISPHub no se pudo leer")
	}
	if ticket.Config.PaperWidthMm == 0 {
		ticket.Config.PaperWidthMm = fallbackPaper
	}
	preview, err := receipt.Preview(ticket)
	if err != nil {
		return Prepared{}, err
	}
	data, err := receipt.Encode(ticket)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Data: data, Preview: preview, Paper: receipt.PaperWidth(ticket.Config)}, nil
}

func prepareBlocks(probe map[string]json.RawMessage, fallbackPaper int) (Prepared, error) {
	raw, ok := probe["document"]
	if !ok {
		return Prepared{}, fmt.Errorf("faltan los bloques del ticket")
	}
	var doc struct {
		Format       string         `json:"format"`
		PaperWidthMm int            `json:"paperWidthMm"`
		Blocks       []escpos.Block `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Prepared{}, fmt.Errorf("los bloques del ticket no se pudieron leer")
	}
	if doc.Format != "ISPHUB_PRINT_V1" {
		return Prepared{}, fmt.Errorf("el formato del ticket tiene que ser ISPHUB_PRINT_V1")
	}
	paper := doc.PaperWidthMm
	if paper == 0 {
		paper = paperFrom(probe, fallbackPaper)
	}
	if paper != 58 {
		paper = 80
	}
	data, err := escpos.Build(paper, doc.Blocks, true)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Data: data, Preview: escpos.Preview(paper, doc.Blocks), Paper: paper}, nil
}

func hasPayment(probe map[string]json.RawMessage) bool {
	_, ok := probe["payment"]
	return ok
}

func paperFrom(probe map[string]json.RawMessage, fallback int) int {
	var paper int
	_ = json.Unmarshal(probe["paperWidthMm"], &paper)
	if paper == 58 {
		return 58
	}
	if paper == 80 {
		return 80
	}
	return fallback
}

func jsonString(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func bytesTrim(body []byte) []byte {
	return []byte(strings.TrimSpace(string(body)))
}
