package launchd

import "strings"
import "testing"

func TestPlistPointsAtServe(t *testing.T) {
	text := Plist("/Applications/ISPHub Print Agent/isphub-print-agent", "/tmp/work", "/tmp/logs")
	for _, want := range []string{
		"app.isphub.print-agent",
		"/Applications/ISPHub Print Agent/isphub-print-agent",
		"<string>serve</string>",
		"/tmp/logs/agent.log",
		"<key>KeepAlive</key>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s\n%s", want, text)
		}
	}
}
