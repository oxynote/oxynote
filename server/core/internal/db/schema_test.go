package db

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _schemaHeader opens schema.sql.
const _schemaHeader = "-- generated from migrations/ by Test_Schema. Do not edit; run `make schema` in server/core.\n\n"

var (
	// _dumpNoise matches the pg_dump lines that carry no schema: comments,
	// session settings and the \restrict guard, whose token is random on
	// every run.
	_dumpNoise = regexp.MustCompile(`(?m)^(--.*|SET .*|SELECT pg_catalog\.set_config\(.*|\\(un)?restrict .*)\n`)

	// _blankRuns matches the blank lines left behind by stripped noise.
	_blankRuns = regexp.MustCompile(`\n{3,}`)

	// _sequenceOwner matches the statement that ties a sequence to the
	// table column it numbers.
	_sequenceOwner = regexp.MustCompile(`^ALTER SEQUENCE public\.(\w+) OWNED BY public\.(\w+)\.`)

	// _firstObject matches the first schema-qualified name in a statement,
	// which is the table or sequence the statement is about.
	_firstObject = regexp.MustCompile(`public\.(\w+)`)
)

func Test_Schema(t *testing.T) {
	db := prepTempDB(t)

	var name string
	require.NoError(t, db.sql.Get(&name, "SELECT current_database()"))

	// the dump runs inside the container, so the schema always comes from
	// the pinned pg_dump and never from one installed on the host.
	var stderr bytes.Buffer

	cmd := exec.CommandContext( //nolint:gosec // args come from the test container
		context.Background(),
		"docker",
		"exec",
		_pgContainerID,
		"pg_dump",
		"--schema-only",
		"--no-owner",
		"--no-privileges",
		"--exclude-table=public.migrations",
		"--username="+_pgUser,
		"--dbname="+name,
	)
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	require.NoError(t, err, stderr.String())

	// the dump is stripped down to its statements, and each table's
	// sequences, constraints and indexes move right after the table.
	// pg_dump separates statements with a blank line, so a function body
	// with blank lines would be split apart.
	dump := _dumpNoise.ReplaceAllString(string(out), "")
	dump = _blankRuns.ReplaceAllString(dump, "\n\n")
	stmts := strings.Split(strings.TrimSpace(dump), "\n\n")

	// OWNED BY comes after CREATE SEQUENCE, so the owners are collected
	// before any statement is grouped.
	owners := make(map[string]string)

	for _, stmt := range stmts {
		if m := _sequenceOwner.FindStringSubmatch(stmt); m != nil {
			owners[m[1]] = m[2]
		}
	}

	var tables []string

	groups := make(map[string][]string)

	for _, stmt := range stmts {
		var table string

		if m := _firstObject.FindStringSubmatch(stmt); m != nil {
			table = m[1]
		}

		if owner, ok := owners[table]; ok {
			table = owner
		}

		if _, ok := groups[table]; !ok {
			tables = append(tables, table)
		}

		groups[table] = append(groups[table], stmt)
	}

	var ordered []string

	for _, table := range tables {
		ordered = append(ordered, groups[table]...)
	}

	got := _schemaHeader + strings.Join(ordered, "\n\n") + "\n"

	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile("schema.sql", []byte(got), 0o600))
		return
	}

	want, err := os.ReadFile("schema.sql")
	require.NoError(t, err)
	assert.Equal(t, string(want), got, "schema.sql is stale: run `make schema` in server/core")
}
