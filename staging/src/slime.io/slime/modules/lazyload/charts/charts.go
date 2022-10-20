package charts

import (
	"embed"
)

const GlobalSidecar = "global-sidecar"

//go:embed all:global-sidecar
var GlobalSidecarFS embed.FS
