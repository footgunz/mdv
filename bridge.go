package main

import "github.com/dgunther/mdv/internal/config"

// cfg bridges the old package-global to the extracted config package.
// Deleted in the cmd/mdv extraction — do not add new readers.
var cfg = config.Default()
