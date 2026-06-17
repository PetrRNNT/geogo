CREATE TABLE IF NOT EXISTS regions (
    id BIGSERIAL PRIMARY KEY,

    country_id BIGINT REFERENCES countries(id),

    name TEXT NOT NULL,

    lat DOUBLE PRECISION,
    lon DOUBLE PRECISION,

    location GEOMETRY(Point, 4326)
);

CREATE INDEX regions_location_idx
ON regions USING GIST(location);