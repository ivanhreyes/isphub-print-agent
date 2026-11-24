// Package netaddr decides which printer addresses the agent is allowed to open.
package netaddr

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// PrivateIPv4 reports whether host is a loopback, link-local, or RFC1918
// address. The agent refuses public hosts so a receipt cannot be pointed at
// a machine outside the shop.
func PrivateIPv4(host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	ip = ip.To4()
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	return false
}

// Port normalizes a printer port. Zero becomes 9100.
func Port(value int) (int, error) {
	if value == 0 {
		return 9100, nil
	}
	if value < 1 || value > 65535 {
		return 0, fmt.Errorf("el puerto %d no es válido", value)
	}
	return value, nil
}

// SplitHostPort accepts "192.168.0.20" or "192.168.0.20:9100".
func SplitHostPort(value string, fallbackPort int) (string, int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", 0, fmt.Errorf("falta la dirección de la impresora")
	}
	host := value
	port := fallbackPort
	if h, p, err := net.SplitHostPort(value); err == nil {
		host = h
		parsed, convErr := strconv.Atoi(p)
		if convErr != nil {
			return "", 0, fmt.Errorf("el puerto de la impresora no es válido")
		}
		port = parsed
	}
	port, err := Port(port)
	if err != nil {
		return "", 0, err
	}
	if !PrivateIPv4(host) {
		return "", 0, fmt.Errorf("la impresora debe estar en una IP privada de la red local")
	}
	return host, port, nil
}
