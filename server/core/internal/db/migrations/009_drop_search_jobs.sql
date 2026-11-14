-- +goose Up

DROP TABLE search_jobs;

-- +goose Down

CREATE TABLE search_jobs (
	id BIGSERIAL PRIMARY KEY,
	version BIGINT NOT NULL DEFAULT 1,
	organization_id TEXT NOT NULL,
	document_id TEXT,
	branch_id TEXT,
	UNIQUE NULLS NOT DISTINCT (organization_id, document_id, branch_id)
);
