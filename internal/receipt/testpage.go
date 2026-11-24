package receipt

import (
	"fmt"
	"time"

	"github.com/ivanhreyes/isphub-print-agent/internal/escpos"
)

// TestPage is a short ticket used to prove the EC-PM-80330, the code page,
// and the cutter without inventing a customer payment.
func TestPage(hostname string, paperWidthMm int) ([]byte, error) {
	if paperWidthMm != 58 {
		paperWidthMm = 80
	}
	when := time.Now().Format("02/01/2006 15:04")
	blocks := []escpos.Block{
		{Type: "text", Text: "ISPHub", Align: "center", Bold: true, Size: "large"},
		{Type: "text", Text: "Agente de impresión", Align: "center"},
		{Type: "text", Text: "EC-PM-80330", Align: "center", Bold: true},
		{Type: "separator"},
		{Type: "text", Text: "Prueba correcta", Align: "center", Bold: true},
		{Type: "text", Text: "áéíóú ñ Ñ ¿¡ €", Align: "center"},
		{Type: "pair", Left: "Mac", Right: hostname},
		{Type: "pair", Left: "Papel", Right: fmt.Sprintf("%d mm", paperWidthMm)},
		{Type: "pair", Left: "Fecha", Right: when},
		{Type: "qr", Value: appDownloadURL},
		{Type: "text", Text: "isphub.app/app", Align: "center"},
	}
	return escpos.Build(paperWidthMm, blocks, true)
}
