package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

var db *sql.DB

func apiKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key != os.Getenv("API_KEY") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный ключ"})
			c.Abort()
			return
		}
		c.Next()
	}
}

type Place struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Country string  `json:"country"`
	City    string  `json:"city"`
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
}

func main() {
	var err error

	// Подключение к базе
	connStr := os.Getenv("DATABASE_URL")
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Ошибка подключения к БД:", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal("БД недоступна:", err)
	}
	log.Println("Подключились к БД!")

	// Роуты
	r := gin.Default()
	r.GET("/places", getPlaces)
	r.GET("/geocode", geocode)
	r.GET("/reverse", reverseGeocode)

	protected := r.Group("/", apiKeyMiddleware())
	protected.POST("/places", addPlace)

	r.Run(":8080")
}

// POST /places — добавить место
func addPlace(c *gin.Context) {
	var p Place
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := db.Exec(`
		INSERT INTO places (name, country, city, address, location)
		VALUES ($1, $2, $3, $4, ST_SetSRID(ST_MakePoint($5, $6), 4326))
	`, p.Name, p.Country, p.City, p.Address, p.Lon, p.Lat)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Место добавлено"})
}

// GET /places — все места
func getPlaces(c *gin.Context) {
	rows, err := db.Query(`
		SELECT id, name, country, city, address,
			ST_Y(location::geometry) AS lat,
			ST_X(location::geometry) AS lon
		FROM places
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var places []Place
	for rows.Next() {
		var p Place
		rows.Scan(&p.ID, &p.Name, &p.Country, &p.City, &p.Address, &p.Lat, &p.Lon)
		places = append(places, p)
	}

	c.JSON(http.StatusOK, places)
}

// GET /geocode?address=Москва — адрес → координаты
func geocode(c *gin.Context) {
	address := c.Query("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Укажи параметр address"})
		return
	}

	var p Place
	err := db.QueryRow(`
		SELECT id, name, country, city, address,
			ST_Y(location::geometry) AS lat,
			ST_X(location::geometry) AS lon
		FROM places
		WHERE address ILIKE $1 OR name ILIKE $1 OR city ILIKE $1
		LIMIT 1
	`, "%"+address+"%").Scan(&p.ID, &p.Name, &p.Country, &p.City, &p.Address, &p.Lat, &p.Lon)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Место не найдено"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, p)
}

// GET /reverse?lat=55.7&lon=37.6 — координаты → адрес
func reverseGeocode(c *gin.Context) {
	lat, err1 := strconv.ParseFloat(c.Query("lat"), 64)
	lon, err2 := strconv.ParseFloat(c.Query("lon"), 64)

	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Укажи lat и lon"})
		return
	}

	var p Place
	err := db.QueryRow(`
		SELECT id, name, country, city, address,
			ST_Y(location::geometry) AS lat,
			ST_X(location::geometry) AS lon
		FROM places
		ORDER BY location <-> ST_SetSRID(ST_MakePoint($1, $2), 4326)
		LIMIT 1
	`, lon, lat).Scan(&p.ID, &p.Name, &p.Country, &p.City, &p.Address, &p.Lat, &p.Lon)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Ничего не найдено"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, p)
}
