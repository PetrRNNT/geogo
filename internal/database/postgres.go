package database

import (
	"database/sql"
	"os"

	_ "github.com/lib/pq"
)

const defaultDSN = "postgres://geo:geopass@localhost:5432/geocoder?sslmode=disable"

func Connect() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = defaultDSN
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}
