package netaddr

import "testing"

func TestPrivateIPv4(t *testing.T) {
	ok := []string{"10.1.2.3", "172.16.0.1", "192.168.1.20", "169.254.1.1", "127.0.0.1"}
	bad := []string{"8.8.8.8", "1.1.1.1", "isphub.app", "::1", ""}
	for _, host := range ok {
		if !PrivateIPv4(host) {
			t.Errorf("expected private: %s", host)
		}
	}
	for _, host := range bad {
		if PrivateIPv4(host) {
			t.Errorf("expected refused: %s", host)
		}
	}
}

func TestSplitHostPort(t *testing.T) {
	host, port, err := SplitHostPort("192.168.1.20:9100", 0)
	if err != nil || host != "192.168.1.20" || port != 9100 {
		t.Fatalf("%s %d %v", host, port, err)
	}
	if _, _, err := SplitHostPort("8.8.8.8", 9100); err == nil {
		t.Fatal("public host was accepted")
	}
}
