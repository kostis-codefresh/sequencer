package ui

import "embed"

// Embedded contains the dashboard pages and static assets.
//
//go:embed *.html *.tmpl dashboard.css img
var Embedded embed.FS
