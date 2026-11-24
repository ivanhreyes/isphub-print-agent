// Package printer sends an ESC/POS buffer to a thermal printer on the LAN.
package printer

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ivanhreyes/isphub-print-agent/internal/netaddr"
)

// Send writes data to host:port and half-closes the connection. EC-PM-80330
// prints the buffer as soon as it arrives; there is no status reply to wait for.
func Send(ctx context.Context, host string, port int, data []byte) error {
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("falta la IP de la impresora")
	}
	host, port, err := netaddr.SplitHostPort(net.JoinHostPort(host, fmt.Sprintf("%d", port)), 9100)
	if err != nil {
		return err
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return fmt.Errorf("no se pudo conectar a la impresora %s:%d: %w", host, port, err)
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(8 * time.Second)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("la impresora no recibió el ticket: %w", err)
	}
	if closer, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = closer.CloseWrite()
	}
	return nil
}
