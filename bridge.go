package main

import (
	"github.com/dgunther/mdv/internal/config"
	"github.com/dgunther/mdv/internal/render"
)

// cfg bridges the old package-global to the extracted config package.
// Deleted in the cmd/mdv extraction — do not add new readers.
var cfg = config.Default()

// renderer bridges the old package-global rendering functions to the
// extracted render package. Deleted in the cmd/mdv extraction.
var renderer = render.Renderer{Cfg: cfg}
