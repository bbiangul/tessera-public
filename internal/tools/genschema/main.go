// Command genschema regenerates api/schemas/*.schema.json from the API types.
// Run it from the repository root: go run ./internal/tools/genschema
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bbiangul/tessera/api/schemas"
)

func main() {
	files, err := schemas.Generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join("api", "schemas", name), data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
