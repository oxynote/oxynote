// Package migrations holds the database migrations written in Go. The SQL
// migrations sit next to them in this directory, and both share one
// numbering.
package migrations

import "github.com/pressly/goose/v3"

// Go returns the Go migrations. Each one is built with goose.NewGoMigration
// from an up and a down step that both use RunTx, and its file is
// NNN_<name>.go in this directory.
func Go() []*goose.Migration {
	return nil
}
