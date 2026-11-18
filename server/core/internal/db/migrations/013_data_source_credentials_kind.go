package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/pressly/goose/v3"
	"github.com/rs/xid"
)

// migrateDataSourceCredentialsKind adds the basic kind to the credentials
// of every data source. Before it, credentials held only a username, a
// password and a CA certificate.
func migrateDataSourceCredentialsKind(deps Deps) *goose.Migration {
	up := func(ctx context.Context, tx *sql.Tx) error {
		return rewriteDataSourceCredentials(ctx, tx, deps.DataSourceKeys, func(creds map[string]any) bool {
			if _, ok := creds["kind"]; ok {
				return false
			}

			creds["kind"] = "basic"

			return true
		})
	}

	// older images know no kind and no token. A token row keeps only its
	// CA certificate, so the user enters a username and password again.
	down := func(ctx context.Context, tx *sql.Tx) error {
		return rewriteDataSourceCredentials(ctx, tx, deps.DataSourceKeys, func(creds map[string]any) bool {
			kind, ok := creds["kind"]
			if !ok {
				return false
			}

			delete(creds, "kind")

			if kind == "token" {
				delete(creds, "token")
			}

			return true
		})
	}

	return goose.NewGoMigration(_versionDataSourceCredentialsKind, &goose.GoFunc{RunTx: up}, &goose.GoFunc{RunTx: down})
}

// rewriteDataSourceCredentials applies edit to the decrypted credentials of
// every data source, and seals those it changed under the newest key.
// Empty credentials stay as they are. Credentials no key reads, and ones
// that are not an object, are reset to empty credentials. So every row has
// the migrated shape, even once a lost key returns.
func rewriteDataSourceCredentials(
	ctx context.Context,
	tx *sql.Tx,
	keys *cryptoutil.Keyring,
	edit func(creds map[string]any) bool,
) error {
	type row struct {
		id          xid.ID
		credentials []byte
	}

	rows, err := tx.QueryContext(ctx, "SELECT id, credentials FROM data_sources")
	if err != nil {
		return fmt.Errorf("selecting data source credentials: %w", err)
	}

	defer rows.Close() //nolint:errcheck // rows.Err reports what went wrong

	var all []row

	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.credentials); err != nil {
			// NOCOV: both columns are NOT NULL and match their types.
			return fmt.Errorf("scanning data source credentials: %w", err)
		}

		all = append(all, r)
	}

	if err := rows.Err(); err != nil {
		// NOCOV: a test cannot break the connection between two rows.
		return fmt.Errorf("reading data source credentials: %w", err)
	}

	for _, r := range all {
		edited, changed, err := editDataSourceCredentials(keys, r.id, r.credentials, edit)
		if err != nil {
			return err
		}

		if !changed {
			continue
		}

		sealed, err := keys.Encrypt(edited, r.id.Bytes())
		if err != nil {
			// NOCOV: crypto/rand failures cannot be simulated in tests.
			return fmt.Errorf("encrypting data source credentials: %w", err)
		}

		if _, err := tx.ExecContext(ctx, "UPDATE data_sources SET credentials = $1 WHERE id = $2", sealed, r.id); err != nil {
			// NOCOV: a test cannot make this statement fail without
			// failing the SELECT above first.
			return fmt.Errorf("updating data source credentials: %w", err)
		}
	}

	return nil
}

// editDataSourceCredentials returns the plaintext the row's credentials
// become, and whether that differs from what is stored. Credentials no
// key reads, and ones that are not an object, become empty.
func editDataSourceCredentials(
	keys *cryptoutil.Keyring,
	id xid.ID,
	sealed []byte,
	edit func(creds map[string]any) bool,
) ([]byte, bool, error) {
	plaintext, err := keys.Decrypt(sealed, id.Bytes())
	if err != nil {
		return nil, true, nil //nolint:nilerr // credentials no key reads are reset, not a failure
	}

	if len(plaintext) == 0 {
		return nil, false, nil
	}

	var creds map[string]any
	if err = json.Unmarshal(plaintext, &creds); err != nil || creds == nil {
		return nil, true, nil //nolint:nilerr // credentials that are not an object are reset, not a failure
	}

	if !edit(creds) {
		return nil, false, nil
	}

	edited, err := json.Marshal(creds)
	if err != nil {
		// NOCOV: values decoded from JSON always encode again.
		return nil, false, fmt.Errorf("marshaling data source credentials: %w", err)
	}

	return edited, true, nil
}
