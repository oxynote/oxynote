-- +migrate Up

-- tags and variable groups store a palette name instead of a hex colour.
-- The hex came from the browser, and browsers round the theme's shades
-- differently, so a stored hex can sit a step off the palette. Each one
-- takes the closest palette colour, and a value that is not a hex takes
-- green, the picker's default.
-- +migrate StatementBegin
CREATE FUNCTION pg_temp.hex_channel(hex TEXT, pos INT) RETURNS INT LANGUAGE SQL IMMUTABLE AS $$
	SELECT ('x' || substr(hex, pos, 2))::BIT(8)::INT
$$;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE FUNCTION pg_temp.palette_name(color TEXT) RETURNS TEXT LANGUAGE SQL IMMUTABLE AS $$
	SELECT CASE WHEN color !~* '^#[0-9a-f]{6}$' THEN 'green' ELSE (
		SELECT p.name
		FROM (VALUES
			('red', '#e7000b'),
			('orange', '#f54a00'),
			('amber', '#e17100'),
			('yellow', '#d08700'),
			('lime', '#5ea500'),
			('green', '#00a63e'),
			('emerald', '#009966'),
			('teal', '#009689'),
			('cyan', '#0092b8'),
			('sky', '#0084d1'),
			('blue', '#155dfc'),
			('indigo', '#4f39f6'),
			('violet', '#7f22fe'),
			('purple', '#9810fa'),
			('fuchsia', '#c800de'),
			('pink', '#e60076')
		) AS p(name, hex)
		ORDER BY
			power(pg_temp.hex_channel(p.hex, 2) - pg_temp.hex_channel(color, 2), 2)
			+ power(pg_temp.hex_channel(p.hex, 4) - pg_temp.hex_channel(color, 4), 2)
			+ power(pg_temp.hex_channel(p.hex, 6) - pg_temp.hex_channel(color, 6), 2)
		LIMIT 1
	) END
$$;
-- +migrate StatementEnd

UPDATE tags SET color = pg_temp.palette_name(color);
UPDATE personal_variable_groups SET color = pg_temp.palette_name(color);

DROP FUNCTION pg_temp.palette_name(TEXT);
DROP FUNCTION pg_temp.hex_channel(TEXT, INT);

-- +migrate Down

-- +migrate StatementBegin
CREATE FUNCTION pg_temp.palette_hex(color TEXT) RETURNS TEXT LANGUAGE SQL IMMUTABLE AS $$
	SELECT p.hex
	FROM (VALUES
		('red', '#e7000b'),
		('orange', '#f54a00'),
		('amber', '#e17100'),
		('yellow', '#d08700'),
		('lime', '#5ea500'),
		('green', '#00a63e'),
		('emerald', '#009966'),
		('teal', '#009689'),
		('cyan', '#0092b8'),
		('sky', '#0084d1'),
		('blue', '#155dfc'),
		('indigo', '#4f39f6'),
		('violet', '#7f22fe'),
		('purple', '#9810fa'),
		('fuchsia', '#c800de'),
		('pink', '#e60076')
	) AS p(name, hex)
	WHERE p.name = color
$$;
-- +migrate StatementEnd

UPDATE tags SET color = pg_temp.palette_hex(color);
UPDATE personal_variable_groups SET color = pg_temp.palette_hex(color);

DROP FUNCTION pg_temp.palette_hex(TEXT);
