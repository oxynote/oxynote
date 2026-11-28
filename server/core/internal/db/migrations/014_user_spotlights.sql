-- +goose Up

CREATE TABLE user_spotlights (
	fk_user_id TEXT PRIMARY KEY REFERENCES users ON DELETE CASCADE,
	star TEXT NULL CHECK (star IN ('shown', 'dismissed'))
);

-- +goose Down

DROP TABLE user_spotlights;
