package agent

import _ "embed"

// setupHTML is the single-file configuration page served at / and /setup.
// It scans the LAN, lists the found printers, and falls back to a manual IP
// form. The page only talks to this same loopback origin.
//
//go:embed setup.html
var setupHTML []byte
