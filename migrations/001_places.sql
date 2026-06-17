CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE IF NOT EXISTS places (
    id BIGSERIAL PRIMARY KEY,

    name    TEXT NOT NULL,
    country TEXT,
    city    TEXT,
    address TEXT,

    lat DOUBLE PRECISION,
    lon DOUBLE PRECISION,

    location GEOMETRY(Point, 4326)
);

CREATE INDEX places_location_idx
ON places USING GIST(location);
