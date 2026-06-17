package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
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

func adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Недостаточно прав"})
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

type User struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password,omitempty"`
	Role     string `json:"role,omitempty"`
}

type Claims struct {
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
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

	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	r.POST("/auth/register", register)
	r.POST("/auth/login", login)
	r.GET("/geocode", geocode)
	r.GET("/reverse", reverseGeocode)
	r.GET("/places", getPlaces)

	// Защищённые роуты для всех авторизованных
	protected := r.Group("/", authMiddleware())
	protected.GET("/me", getMe)

	// Только для админа
	admin := r.Group("/", authMiddleware(), adminMiddleware())
	admin.POST("/places", addPlace)
	admin.DELETE("/places/:id", deletePlace)

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

// POST /auth/register
func register(c *gin.Context) {
	var u User
	if err := c.ShouldBindJSON(&u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка хэширования"})
		return
	}

	var id int
	err = db.QueryRow(`
        INSERT INTO users (email, password) VALUES ($1, $2) RETURNING id
    `, u.Email, string(hash)).Scan(&id)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email уже занят"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Пользователь создан", "id": id})
}

// POST /auth/login
func login(c *gin.Context) {
	var u User
	if err := c.ShouldBindJSON(&u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var dbUser User
	err := db.QueryRow(`
        SELECT id, email, password, role FROM users WHERE email = $1
    `, u.Email).Scan(&dbUser.ID, &dbUser.Email, &dbUser.Password, &dbUser.Role)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный email или пароль"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(dbUser.Password), []byte(u.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный email или пароль"})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: dbUser.ID,
		Email:  dbUser.Email,
		Role:   dbUser.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	})

	signed, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания токена"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": signed, "email": dbUser.Email, "role": dbUser.Role})
}

func getMe(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"id":    c.GetInt("user_id"),
		"email": c.GetString("email"),
		"role":  c.GetString("role"),
	})
}

func deletePlace(c *gin.Context) {
	id := c.Param("id")
	_, err := db.Exec(`DELETE FROM places WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Место удалено"})
}

// Middleware проверки JWT
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || len(header) < 8 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Токен не передан"})
			c.Abort()
			return
		}

		tokenStr := header[7:] // убираем "Bearer "
		claims := &Claims{}

		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_SECRET")), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный или истёкший токен"})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)
		c.Next()
	}
}
