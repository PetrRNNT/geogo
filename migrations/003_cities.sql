CREATE TABLE IF NOT EXISTS cities (
    id BIGSERIAL PRIMARY KEY,

    country_id BIGINT REFERENCES countries(id),

    name TEXT NOT NULL,

    lat DOUBLE PRECISION,
    lon DOUBLE PRECISION,

    location GEOMETRY(Point, 4326)
);

CREATE INDEX cities_location_idx
ON cities USING GIST(location);