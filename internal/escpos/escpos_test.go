package escpos

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildSpanishAndCut(t *testing.T) {
	data, err := Build(80, []Block{
		{Type: "text", Text: "Señal y año", Align: "center", Bold: true},
		{Type: "pair", Left: "Total", Right: "$10.00"},
		{Type: "qr", Value: "https://isphub.app/app"},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0x1b, '@', 0x1b, 't', 19}) {
		t.Fatalf("missing init and PC858 select: %x", data[:8])
	}
	if !bytes.Contains(data, []byte{164}) { // ñ
		t.Fatal("expected PC858 ñ")
	}
	if !bytes.Contains(data, []byte("https://isphub.app/app")) {
		t.Fatal("QR payload missing")
	}
	if !bytes.HasSuffix(data, []byte{0x1d, 'V', 66, 0}) {
		t.Fatalf("missing partial cut: %x", data[len(data)-4:])
	}
	if Columns(58) != 32 || Columns(80) != 48 || Columns(0) != 48 {
		t.Fatal("unexpected columns")
	}
}

func TestPairTruncatesLeft(t *testing.T) {
	line := pairLine("Concepto muy largo de factura", "$1.00", 16)
	if len([]rune(line)) != 16 {
		t.Fatalf("width %d: %q", len([]rune(line)), line)
	}
	if !strings.HasSuffix(line, "$1.00") {
		t.Fatal(line)
	}
}

func TestPreviewWraps(t *testing.T) {
	text := Preview(58, []Block{{Type: "text", Text: "uno dos tres cuatro cinco seis siete ocho nueve diez"}})
	if !strings.Contains(text, "\n") {
		t.Fatal(text)
	}
}

func TestRejectsEmptyQR(t *testing.T) {
	_, err := Build(80, []Block{{Type: "qr", Value: "  "}}, false)
	if err == nil {
		t.Fatal("expected error")
	}
}
