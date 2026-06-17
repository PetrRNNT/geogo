CREATE TABLE IF NOT EXISTS pois (
    id BIGSERIAL PRIMARY KEY,

    city_id BIGINT REFERENCES cities(id),

    name TEXT NOT NULL,
    address TEXT,

    lat DOUBLE PRECISION,
    lon DOUBLE PRECISION,

    location GEOMETRY(Point, 4326)
);

CREATE INDEX pois_location_idx
ON pois USING GIST(location);