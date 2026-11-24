package printer

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestSendWritesTheBuffer(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			got <- nil
			return
		}
		defer conn.Close()
		buf, _ := io.ReadAll(conn)
		got <- buf
	}()
	payload := []byte{0x1b, '@', 'H', 'o', 'l', 'a'}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, portText, _ := net.SplitHostPort(ln.Addr().String())
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	if err := Send(ctx, "127.0.0.1", port, payload); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-got:
		if string(body) != string(payload) {
			t.Fatalf("got %x", body)
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

func TestSendRejectsPublicHost(t *testing.T) {
	err := Send(context.Background(), "8.8.8.8", 9100, []byte{0x1b, '@'})
	if err == nil {
		t.Fatal("public host was contacted")
	}
}
