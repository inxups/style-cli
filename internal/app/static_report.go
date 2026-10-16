package app

import (
	"fmt"
	"io"

	"github.com/inxups/style-cli/internal/adapter"
)

func writeStaticMigrations(out io.Writer, migrations []adapter.StaticMigration) {
	for _, m := range migrations {
		to := m.To
		if to == "" {
			to = "removed"
		}
		fmt.Fprintf(out, "static migration: %s %s: %s -> %s\n", m.Harness, m.Package, m.From, to)
	}
}
