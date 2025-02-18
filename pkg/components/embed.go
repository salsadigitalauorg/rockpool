package components

import "embed"

//go:embed default/*.yaml
var DefaultComponents embed.FS
