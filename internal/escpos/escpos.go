// Package escpos builds ESC/POS bytes for an 80 mm or 58 mm thermal head.
// The EC Line EC-PM-80330 accepts this dialect on TCP port 9100: PC858 for
// Spanish, Epson QR, and a partial cut.
package escpos

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	esc byte = 0x1b
	gs  byte = 0x1d
	lf  byte = 0x0a
)

// Columns is the Font A width of a 203 dpi head. 58 mm paper prints 32
// characters; 80 mm paper prints 48. Any other width is treated as 80 mm.
func Columns(paperWidthMm int) int {
	if paperWidthMm == 58 {
		return 32
	}
	return 48
}

// Block is one visual row of an ISPHUB_PRINT_V1 document.
type Block struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Left  string `json:"left,omitempty"`
	Right string `json:"right,omitempty"`
	Align string `json:"align,omitempty"`
	Bold  bool   `json:"bold,omitempty"`
	Size  string `json:"size,omitempty"`
	Char  string `json:"char,omitempty"`
	Lines int    `json:"lines,omitempty"`
	Value string `json:"value,omitempty"`
}

// Build encodes blocks and, when cut is true, feeds three lines and sends a
// partial cut (GS V 66 0).
func Build(paperWidthMm int, blocks []Block, cut bool) ([]byte, error) {
	if len(blocks) > 400 {
		return nil, fmt.Errorf("el ticket tiene demasiados bloques")
	}
	cols := Columns(paperWidthMm)
	var buf bytes.Buffer
	buf.Write([]byte{esc, '@'})
	buf.Write([]byte{esc, 't', 19}) // PC858: ñ, acentos y euro.
	for _, block := range blocks {
		if err := writeBlock(&buf, cols, block); err != nil {
			return nil, err
		}
	}
	if cut {
		buf.Write([]byte{lf, lf, lf})
		buf.Write([]byte{gs, 'V', 66, 0})
	}
	return buf.Bytes(), nil
}

// Preview renders the same blocks as plain text for a dry run.
func Preview(paperWidthMm int, blocks []Block) string {
	cols := Columns(paperWidthMm)
	var lines []string
	for _, block := range blocks {
		switch block.Type {
		case "text":
			width := cols
			if block.Size == "large" {
				width = cols / 2
			}
			for _, row := range wrap(block.Text, width) {
				lines = append(lines, place(row, width, block.Align))
			}
		case "pair":
			lines = append(lines, pairLine(block.Left, block.Right, cols))
		case "separator":
			ch := "-"
			if block.Char == "=" {
				ch = "="
			}
			lines = append(lines, strings.Repeat(ch, cols))
		case "qr":
			lines = append(lines, "[QR "+strings.TrimSpace(block.Value)+"]")
		case "feed":
			n := block.Lines
			if n < 0 {
				n = 0
			}
			if n > 8 {
				n = 8
			}
			for i := 0; i < n; i++ {
				lines = append(lines, "")
			}
		}
	}
	return strings.Join(lines, "\n")
}

func writeBlock(buf *bytes.Buffer, cols int, block Block) error {
	switch block.Type {
	case "text":
		writeText(buf, cols, block)
	case "pair":
		resetStyle(buf)
		buf.Write(encode(pairLine(block.Left, block.Right, cols)))
		buf.WriteByte(lf)
	case "separator":
		resetStyle(buf)
		ch := "-"
		if block.Char == "=" {
			ch = "="
		}
		buf.Write(encode(strings.Repeat(ch, cols)))
		buf.WriteByte(lf)
	case "qr":
		return writeQR(buf, block.Value, cols)
	case "feed":
		n := block.Lines
		if n < 0 {
			n = 0
		}
		if n > 8 {
			n = 8
		}
		buf.Write(bytes.Repeat([]byte{lf}, n))
	case "":
		return fmt.Errorf("bloque sin tipo")
	default:
		return fmt.Errorf("bloque %q no está soportado", block.Type)
	}
	return nil
}

func writeText(buf *bytes.Buffer, cols int, block Block) {
	width := cols
	size := byte(0)
	if block.Size == "large" {
		width = cols / 2
		if width < 1 {
			width = 1
		}
		size = 0x11
	}
	align := byte(0)
	switch block.Align {
	case "center":
		align = 1
	case "right":
		align = 2
	}
	buf.Write([]byte{esc, 'a', align})
	bold := byte(0)
	if block.Bold {
		bold = 1
	}
	buf.Write([]byte{esc, 'E', bold})
	buf.Write([]byte{gs, '!', size})
	rows := wrap(block.Text, width)
	if len(rows) == 0 {
		rows = []string{""}
	}
	for _, row := range rows {
		buf.Write(encode(row))
		buf.WriteByte(lf)
	}
	resetStyle(buf)
}

func resetStyle(buf *bytes.Buffer) {
	buf.Write([]byte{esc, 'a', 0})
	buf.Write([]byte{esc, 'E', 0})
	buf.Write([]byte{gs, '!', 0})
}

func writeQR(buf *bytes.Buffer, value string, cols int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("el código QR está vacío")
	}
	if len(value) > 512 {
		return fmt.Errorf("el código QR es demasiado largo")
	}
	module := byte(5)
	if cols <= 32 {
		module = 4
	}
	data := []byte(value)
	storeLen := len(data) + 3
	resetStyle(buf)
	buf.Write([]byte{esc, 'a', 1})
	// Model 2, module size, error correction M, store, print.
	buf.Write([]byte{gs, '(', 'k', 4, 0, 49, 65, 50, 0})
	buf.Write([]byte{gs, '(', 'k', 3, 0, 49, 67, module})
	buf.Write([]byte{gs, '(', 'k', 3, 0, 49, 69, 49})
	buf.Write([]byte{gs, '(', 'k', byte(storeLen), byte(storeLen >> 8), 49, 80, 48})
	buf.Write(data)
	buf.Write([]byte{gs, '(', 'k', 3, 0, 49, 81, 48})
	buf.WriteByte(lf)
	resetStyle(buf)
	return nil
}

func pairLine(left, right string, cols int) string {
	left = clean(left)
	right = clean(right)
	if cols < 1 {
		cols = 1
	}
	lr := []rune(left)
	rr := []rune(right)
	if len(rr) >= cols {
		return string(rr[:cols])
	}
	room := cols - len(rr) - 1
	if room < 1 {
		return string(rr)
	}
	if len(lr) > room {
		if room == 1 {
			lr = lr[:1]
		} else {
			lr = append(lr[:room-1], '…')
		}
	}
	gap := cols - len(lr) - len(rr)
	return string(lr) + strings.Repeat(" ", gap) + string(rr)
}

func place(row string, cols int, align string) string {
	runes := []rune(row)
	if len(runes) >= cols {
		return string(runes[:cols])
	}
	pad := cols - len(runes)
	switch align {
	case "center":
		left := pad / 2
		return strings.Repeat(" ", left) + row + strings.Repeat(" ", pad-left)
	case "right":
		return strings.Repeat(" ", pad) + row
	default:
		return row
	}
}

func wrap(value string, cols int) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if cols < 1 {
		cols = 1
	}
	var rows []string
	for _, paragraph := range strings.Split(value, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			rows = append(rows, "")
			continue
		}
		var line []rune
		flush := func() {
			if len(line) == 0 {
				return
			}
			rows = append(rows, string(line))
			line = nil
		}
		for _, word := range words {
			wr := []rune(word)
			for len(wr) > cols {
				flush()
				rows = append(rows, string(wr[:cols]))
				wr = wr[cols:]
			}
			if len(wr) == 0 {
				continue
			}
			if len(line) == 0 {
				line = wr
				continue
			}
			if len(line)+1+len(wr) > cols {
				flush()
				line = wr
				continue
			}
			line = append(line, ' ')
			line = append(line, wr...)
		}
		flush()
	}
	return rows
}

func clean(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	return strings.TrimSpace(value)
}

func encode(value string) []byte {
	replacer := strings.NewReplacer(
		"–", "-", "—", "-", "‘", "'", "’", "'", "“", "\"", "”", "\"",
		"•", "*", "·", "-", "…", "...",
	)
	value = replacer.Replace(value)
	out := make([]byte, 0, len(value))
	for _, r := range value {
		switch {
		case r == '\n' || r == '\r':
			continue
		case r < 128:
			out = append(out, byte(r))
		case cp858[r] != 0:
			out = append(out, cp858[r])
		default:
			if folded, ok := foldLatin(r); ok {
				out = append(out, folded)
			} else {
				out = append(out, '?')
			}
		}
	}
	return out
}

// cp858 maps the Spanish and currency characters this printer prints when
// ESC t 19 is selected. Zero is not a used byte in this subset.
var cp858 = map[rune]byte{
	'Ç': 128, 'ü': 129, 'é': 130, 'â': 131, 'ä': 132, 'à': 133,
	'å': 134, 'ç': 135, 'ê': 136, 'ë': 137, 'è': 138, 'ï': 139,
	'î': 140, 'ì': 141, 'Ä': 142, 'Å': 143, 'É': 144, 'æ': 145,
	'Æ': 146, 'ô': 147, 'ö': 148, 'ò': 149, 'û': 150, 'ù': 151,
	'ÿ': 152, 'Ö': 153, 'Ü': 154, 'ø': 155, '£': 156, 'Ø': 157,
	'×': 158, 'ƒ': 159, 'á': 160, 'í': 161, 'ó': 162, 'ú': 163,
	'ñ': 164, 'Ñ': 165, 'ª': 166, 'º': 167, '¿': 168,
	'¡': 173, 'Á': 181, 'Í': 214, 'Ó': 224, 'Ú': 233, '€': 213,
}

func foldLatin(r rune) (byte, bool) {
	switch r {
	case 'á', 'à', 'ä', 'â', 'å', 'Á', 'À', 'Ä', 'Â':
		return 'a', true
	case 'é', 'è', 'ë', 'ê', 'É', 'È', 'Ë', 'Ê':
		return 'e', true
	case 'í', 'ì', 'ï', 'î', 'Í', 'Ì', 'Ï', 'Î':
		return 'i', true
	case 'ó', 'ò', 'ö', 'ô', 'Ó', 'Ò', 'Ö', 'Ô':
		return 'o', true
	case 'ú', 'ù', 'ü', 'û', 'Ú', 'Ù', 'Ü', 'Û':
		return 'u', true
	case 'ñ':
		return 'n', true
	case 'Ñ':
		return 'N', true
	case 'ç', 'Ç':
		return 'c', true
	default:
		return 0, false
	}
}
