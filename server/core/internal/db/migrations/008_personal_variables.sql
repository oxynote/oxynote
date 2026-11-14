-- +goose Up

CREATE TABLE personal_variable_groups (
	id TEXT PRIMARY KEY,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	group_name TEXT NOT NULL,
	color TEXT NOT NULL,
	sort_index INTEGER NOT NULL,
	builtin BOOLEAN NOT NULL,
	created_at TIMESTAMP NOT NULL,
	fk_created_by TEXT REFERENCES users ON DELETE SET NULL,
	UNIQUE (fk_organization_id, group_name)
);
CREATE INDEX personal_variable_groups_fk_created_by_idx ON personal_variable_groups (fk_created_by);

-- the group key has no cascade, so a group that still holds variables
-- cannot be deleted.
CREATE TABLE personal_variables (
	id TEXT PRIMARY KEY,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_variable_group_id TEXT NOT NULL REFERENCES personal_variable_groups,
	variable_key TEXT NOT NULL,
	variable_name TEXT NOT NULL,
	description TEXT,
	variable_type TEXT NOT NULL CHECK (variable_type IN ('list', 'number', 'text', 'time-range')),
	default_value TEXT,
	sort_index INTEGER NOT NULL,
	builtin BOOLEAN NOT NULL,
	created_at TIMESTAMP NOT NULL,
	fk_created_by TEXT REFERENCES users ON DELETE SET NULL,
	UNIQUE (fk_organization_id, variable_key)
);
CREATE INDEX personal_variables_fk_variable_group_id_idx ON personal_variables (fk_variable_group_id);
CREATE INDEX personal_variables_fk_created_by_idx ON personal_variables (fk_created_by);

CREATE TABLE personal_variable_options (
	id TEXT PRIMARY KEY,
	fk_variable_id TEXT NOT NULL REFERENCES personal_variables ON DELETE CASCADE,
	option_value TEXT NOT NULL,
	sort_index INTEGER NOT NULL
);
CREATE INDEX personal_variable_options_fk_variable_id_idx ON personal_variable_options (fk_variable_id);

CREATE TABLE personal_variable_values (
	fk_variable_id TEXT NOT NULL REFERENCES personal_variables ON DELETE CASCADE,
	fk_user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	value TEXT NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	PRIMARY KEY (fk_variable_id, fk_user_id)
);
CREATE INDEX personal_variable_values_fk_user_id_idx ON personal_variable_values (fk_user_id);
CREATE INDEX personal_variable_values_fk_organization_id_idx ON personal_variable_values (fk_organization_id);

-- an xid is 12 bytes in 20 base32hex characters, so the last character
-- carries a single bit and must be 0 or g.
-- +goose StatementBegin
CREATE FUNCTION pg_temp.random_xid() RETURNS TEXT LANGUAGE SQL VOLATILE AS $$
	SELECT string_agg(substr('0123456789abcdefghijklmnopqrstuv', floor(random() * 32)::INT + 1, 1), '')
		|| substr('0g', floor(random() * 2)::INT + 1, 1)
	FROM generate_series(1, 19)
$$;
-- +goose StatementEnd

-- every existing organization gets the built-ins a new one is seeded with.
-- +goose StatementBegin
DO $$
DECLARE
	org RECORD;
	group_id TEXT;
	refresh_id TEXT;
BEGIN
	FOR org IN SELECT id FROM organizations LOOP
		group_id := pg_temp.random_xid();
		refresh_id := pg_temp.random_xid();

		INSERT INTO personal_variable_groups (id, fk_organization_id, group_name, color, sort_index, builtin, created_at)
		VALUES (group_id, org.id, 'Metrics', '#155dfc', 0, TRUE, NOW());

		INSERT INTO personal_variables (id, fk_organization_id, fk_variable_group_id, variable_key, variable_name, variable_type, default_value, sort_index, builtin, created_at)
		VALUES
			(pg_temp.random_xid(), org.id, group_id, 'metric_range', 'Metric time range', 'time-range', 'last_24_hours', 0, TRUE, NOW()),
			(refresh_id, org.id, group_id, 'metric_refresh', 'Metric refresh interval', 'list', NULL, 1, TRUE, NOW());

		INSERT INTO personal_variable_options (id, fk_variable_id, option_value, sort_index)
		SELECT pg_temp.random_xid(), refresh_id, o.value, o.position - 1
		FROM unnest(ARRAY['5m', '5s', '10s', '30s', '1m', '15m', '30m', '1h', '2h', '1d']) WITH ORDINALITY AS o(value, position);

		UPDATE personal_variables SET default_value = (
			SELECT id FROM personal_variable_options WHERE fk_variable_id = refresh_id AND sort_index = 0
		) WHERE id = refresh_id;
	END LOOP;
END
$$;
-- +goose StatementEnd

DROP FUNCTION pg_temp.random_xid();

-- +goose Down

DROP TABLE personal_variable_values;
DROP TABLE personal_variable_options;
DROP TABLE personal_variables;
DROP TABLE personal_variable_groups;
