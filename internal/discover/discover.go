// Package discover finds EC-PM-80330 class printers on the local IPv4 network.
// Identification uses the open print port and the printer's own web page.
// It never sends a test ticket.
package discover

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ivanhreyes/isphub-print-agent/internal/netaddr"
)

const defaultPort = 9100

// Printer is one candidate found on the LAN.
type Printer struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	MAC         string `json:"mac,omitempty"`
	Model       string `json:"model,omitempty"`
	Label       string `json:"label"`
	Role        string `json:"role"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Scan looks for TCP 9100 on each local /24 and classifies the responders.
func Scan(ctx context.Context) ([]Printer, error) {
	hosts, err := hostsToScan()
	if err != nil {
		return nil, err
	}
	open := probePorts(ctx, hosts, defaultPort, 48, 250*time.Millisecond)
	printers := make([]Printer, 0, len(open))
	for _, host := range open {
		printers = append(printers, fingerprint(ctx, host, defaultPort))
	}
	sort.Slice(printers, func(i, j int) bool { return printers[i].Host < printers[j].Host })
	return printers, nil
}

func hostsToScan() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var hosts []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil || !netaddr.PrivateIPv4(ipnet.IP.String()) {
				continue
			}
			ip := ipnet.IP.To4()
			// Clip anything wider than /24 to the host's own /24.
			base := ip.Mask(net.CIDRMask(24, 32))
			for n := 1; n <= 254; n++ {
				candidate := net.IPv4(base[0], base[1], base[2], byte(n)).String()
				if candidate == ip.String() || seen[candidate] {
					continue
				}
				seen[candidate] = true
				hosts = append(hosts, candidate)
			}
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("esta Mac no tiene una red IPv4 privada activa")
	}
	if len(hosts) > 1024 {
		hosts = hosts[:1024]
	}
	return hosts, nil
}

func probePorts(ctx context.Context, hosts []string, port int, concurrency int, timeout time.Duration) []string {
	if concurrency < 1 {
		concurrency = 1
	}
	var mu sync.Mutex
	var open []string
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		sem <- struct{}{}
		go func(host string) {
			defer wg.Done()
			defer func() { <-sem }()
			dialer := net.Dialer{Timeout: timeout}
			conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
			if err != nil {
				return
			}
			conn.Close()
			mu.Lock()
			open = append(open, host)
			mu.Unlock()
		}(host)
	}
	wg.Wait()
	return open
}

func fingerprint(ctx context.Context, host string, port int) Printer {
	body, server := readHTTP(ctx, host)
	role := classify(body, server)
	model := ""
	// Status bytes are only for ESC/POS heads. A label printer must not receive them.
	if role != "label" {
		model = usefulModel(readModel(ctx, host, port))
	}
	label := "Impresora " + host
	switch role {
	case "ticket":
		label = "EC-PM-80330 · tickets"
		if model != "" {
			label = model + " · tickets"
		}
	case "label":
		label = "Impresora de etiquetas"
	}
	fp := fingerprintText(body, server, model)
	return Printer{
		Host:        host,
		Port:        port,
		Model:       model,
		Label:       label,
		Role:        role,
		Fingerprint: fp,
	}
}

func classify(body, server string) string {
	haystack := strings.ToLower(body + " " + server)
	if strings.Contains(haystack, "brother") || strings.Contains(haystack, "debut") || strings.Contains(haystack, "ql-") {
		return "label"
	}
	title := strings.ToLower(pageTitle(body))
	if strings.Contains(title, "configures") && strings.Contains(haystack, "gb2312") {
		return "ticket"
	}
	return "unknown"
}

func fingerprintText(body, server, model string) string {
	parts := []string{}
	if title := pageTitle(body); title != "" {
		parts = append(parts, "title="+strings.ToLower(title))
	}
	if strings.Contains(strings.ToLower(body), "gb2312") {
		parts = append(parts, "charset=gb2312")
	}
	if server != "" {
		parts = append(parts, "server="+strings.ToLower(server))
	}
	if model != "" {
		parts = append(parts, "model="+strings.ToLower(model))
	}
	return strings.Join(parts, ";")
}

var titlePattern = regexp.MustCompile(`(?i)<title>([^<]*)</title>`)

func pageTitle(body string) string {
	match := titlePattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func readHTTP(ctx context.Context, host string) (string, string) {
	reqCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://"+host+"/", nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Connection", "close")
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	return string(buf), resp.Header.Get("Server")
}

func readModel(ctx context.Context, host string, port int) string {
	dialer := net.Dialer{Timeout: 800 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(800 * time.Millisecond))
	// DLE EOT 1 asks for status. GS I 69 asks for the printer name.
	_, _ = conn.Write([]byte{0x10, 0x04, 0x01, 0x1d, 0x49, 0x45})
	buf := make([]byte, 64)
	n, _ := conn.Read(buf)
	return printableModel(buf[:n])
}

func usefulModel(model string) string {
	upper := strings.ToUpper(model)
	if strings.Contains(upper, "CHINESE") || strings.Contains(upper, "GB18030") || strings.Contains(upper, "GB2312") {
		return ""
	}
	return model
}

func printableModel(raw []byte) string {
	var b strings.Builder
	for _, c := range raw {
		if c >= 0x20 && c <= 0x7e {
			b.WriteByte(c)
		} else if b.Len() > 0 {
			b.WriteByte(' ')
		}
	}
	text := strings.TrimSpace(b.String())
	if text == "" || !strings.ContainsFunc(text, func(r rune) bool { return r >= 'A' && r <= 'z' }) {
		return ""
	}
	if len(text) > 40 {
		text = text[:40]
	}
	return text
}
