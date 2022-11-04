package charts

import (
	"embed"
)

const GlobalSidecar = "global-sidecar"

//go:embed global-sidecar
var GlobalSidecarFS embed.FS
