# Геокодер на Go — итоги сессии

## Что построили

REST API геокодер на Go с PostgreSQL + PostGIS, задеплоенный на Railway.

```
Vue фронт (в будущем)
      ↓
Go API (Gin) — :8080
      ↓
PostgreSQL + PostGIS
```

---

## Стек

| Компонент | Технология |
|-----------|-----------|
| Язык | Go |
| HTTP фреймворк | Gin |
| База данных | PostgreSQL 17 + PostGIS |
| Локальная разработка | Docker Compose |
| Деплой | Railway |

---

## Структура проекта

```
geocoder/
├── main.go
├── go.mod
├── go.sum
└── docker-compose.yml
```

---

## docker-compose.yml

```yaml
services:
  db:
    image: postgis/postgis:16-3.4
    environment:
      POSTGRES_DB: geocoder
      POSTGRES_USER: geo
      POSTGRES_PASSWORD: geopass
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
```

---

## SQL — создание таблицы

```sql
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE places (
    id         SERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    country    TEXT,
    city       TEXT,
    address    TEXT,
    location   GEOMETRY(Point, 4326),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX places_location_idx ON places USING GIST(location);
```

`GEOMETRY(Point, 4326)` — хранит координаты в формате PostGIS.  
`GIST` индекс — специальный пространственный индекс для быстрого поиска по координатам.

---

## Эндпоинты API

| Метод | URL | Описание |
|-------|-----|----------|
| `POST` | `/places` | Добавить место (защищён API ключом) |
| `GET` | `/places` | Список всех мест |
| `GET` | `/geocode?address=Москва` | Адрес → координаты |
| `GET` | `/reverse?lat=55.7&lon=37.6` | Координаты → адрес |

---

## main.go

```go
package main

import (
    "database/sql"
    "log"
    "net/http"
    "strconv"
    "os"

    "github.com/gin-gonic/gin"
    _ "github.com/lib/pq"
)

var db *sql.DB

type Place struct {
    ID      int     `json:"id"`
    Name    string  `json:"name"`
    Country string  `json:"country"`
    City    string  `json:"city"`
    Address string  `json:"address"`
    Lat     float64 `json:"lat"`
    Lon     float64 `json:"lon"`
}

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

func main() {
    var err error

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

    r := gin.Default()

    // Открытые эндпоинты
    r.GET("/geocode", geocode)
    r.GET("/reverse", reverseGeocode)
    r.GET("/places", getPlaces)

    // Защищённые эндпоинты
    protected := r.Group("/", apiKeyMiddleware())
    protected.POST("/places", addPlace)

    r.Run(":8080")
}

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
```

---

## Деплой на Railway

1. Код на GitHub
2. Railway → New Project → Deploy from GitHub repo
3. Добавить сервис **PostGIS 17** из шаблонов
4. Создать таблицу через Console сервиса PostGIS
5. В сервисе geogo → Variables добавить:
   - `DATABASE_URL` — из Variables сервиса PostGIS
   - `API_KEY` — любой секретный ключ

Railway автоматически деплоит при каждом `git push`.

---

## Тестирование API

```bash
# Добавить место (с ключом)
curl -X POST https://geogo-production.up.railway.app/places \
  -H "Content-Type: application/json" \
  -H "X-API-Key: твой_ключ" \
  -d '{"name": "Кремль", "country": "Russia", "city": "Moscow", "address": "Красная площадь, 1", "lat": 55.7520, "lon": 37.6175}'

# Все места
curl https://geogo-production.up.railway.app/places

# Геокодинг
curl "https://geogo-production.up.railway.app/geocode?address=Кремль"

# Обратный геокодинг
curl "https://geogo-production.up.railway.app/reverse?lat=55.75&lon=37.61"
```

---

## Что можно улучшить

- Возвращать топ-5 результатов вместо одного
- Добавить расстояние в метрах в ответ `/reverse`
- Добавить `PUT /places/:id`
- Вынести конфиг в отдельный файл
- Написать тесты
- Импорт данных из OpenStreetMap

---

## Vue фронт

Стек: Vue 3, Leaflet, Axios, тёмная тема.

### Структура

```
geocoder-front/
├── src/
│   ├── App.vue
│   ├── api.js
│   ├── stores/
│   │   └── auth.js
│   └── views/
│       ├── AuthView.vue
│       ├── SearchView.vue
│       ├── AddPlaceView.vue
│       └── PlacesView.vue
├── .env
└── package.json
```

### .env

```
VITE_API_URL=https://geogo-production.up.railway.app
```

### Вкладки

| Вкладка | Кто видит | Что делает |
|---------|-----------|-----------|
| Поиск | Все | Адрес → координаты, координаты → адрес, карта |
| Все места | Все | Список мест с фильтром, детальная панель |
| Добавить место | Только админ | Форма + клик по карте |

### Карта

Используется Leaflet + тёмные тайлы CartoCDN:
```javascript
L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png')
```

---

## JWT авторизация

### Таблица users

```sql
CREATE TABLE users (
    id         SERIAL PRIMARY KEY,
    email      TEXT UNIQUE NOT NULL,
    password   TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'user',
    created_at TIMESTAMP DEFAULT NOW()
);
```

### Роли

| Роль | Права |
|------|-------|
| `user` | Поиск, просмотр мест |
| `admin` | Всё выше + добавление и удаление мест |

Сделать себя админом:
```sql
UPDATE users SET role = 'admin' WHERE email = 'твой@email.com';
```

### Переменные окружения на Railway

```
JWT_SECRET = длинная_случайная_строка
```

### Эндпоинты авторизации

| Метод | URL | Описание |
|-------|-----|----------|
| `POST` | `/auth/register` | Регистрация |
| `POST` | `/auth/login` | Вход, возвращает JWT токен |
| `GET` | `/me` | Данные текущего пользователя |

### Защита роутов

```go
// Публичные
r.GET("/geocode", geocode)
r.GET("/reverse", reverseGeocode)
r.GET("/places", getPlaces)
r.POST("/auth/register", register)
r.POST("/auth/login", login)

// Для авторизованных
protected := r.Group("/", authMiddleware())
protected.GET("/me", getMe)

// Только для админа
admin := r.Group("/", authMiddleware(), adminMiddleware())
admin.POST("/places", addPlace)
admin.DELETE("/places/:id", deletePlace)
```

### Токен на фронте

Токен хранится в `localStorage` и автоматически добавляется в каждый запрос через axios interceptor:

```javascript
api.interceptors.request.use((config) => {
  if (auth.token) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  return config
})
```

---

## Начальные данные

20 знаковых мест по всему миру — Кремль, Эйфелева башня, Колизей, Тадж-Махал и другие. Залиты через консоль Railway в PostGIS.

---

## Деплой фронта на Railway

1. GitHub репозиторий с Vue проектом
2. Railway → New Service → GitHub repo
3. Variables: `VITE_API_URL`
4. Start command: `npx serve dist --listen $PORT`
5. Networking → порт `8080` → Generate Domain
