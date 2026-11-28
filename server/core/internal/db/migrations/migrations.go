// Package migrations holds the database migrations written in Go. The SQL
// migrations sit next to them in this directory, and both share one
// numbering.
package migrations

import (
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/pressly/goose/v3"
)

// All Go migration versions.
const (
	// _versionDataSourceCredentialsKind adds a kind to data source
	// credentials.
	_versionDataSourceCredentialsKind version = 13

	// _versionDocumentCommentRevisionsBackfill gives existing comments
	// and replies their first revision.
	_versionDocumentCommentRevisionsBackfill version = 15

	// _versionDocumentActivityEntriesBackfill rebuilds the activity
	// entries that existing rows can tell.
	_versionDocumentActivityEntriesBackfill version = 17
)

// version is the number a Go migration shares with the SQL ones.
type version = int64

// Deps holds what the Go migrations need besides their transaction.
type Deps struct {
	// DataSourceKeys holds the keys data source credentials are sealed
	// with.
	DataSourceKeys *cryptoutil.Keyring
}

// Go returns the Go migrations. Each one is built with goose.NewGoMigration
// from an up and a down step that both use RunTx, and its file is
// NNN_<name>.go in this directory.
func Go(deps Deps) []*goose.Migration {
	return []*goose.Migration{
		migrateDataSourceCredentialsKind(deps),
		backfillDocumentCommentRevisions(),
		backfillDocumentActivityEntries(),
	}
}
