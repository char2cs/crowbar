//go:build !noEmbed

package main

import "embed"

//go:embed all:web/dist
var embeddedWeb embed.FS
