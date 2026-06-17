# GEOGO — текущая документация geocoder

## Текущее состояние

Проект — REST API геокодера на Go с Gin, PostgreSQL и PostGIS.

В репозитории есть два слоя состояния:

1. Рабочий HTTP-сервер в `main.go`: регистрация, логин, JWT, `/geocode`, `/reverse`, `/places`, добавление и удаление мест.
2. Начатый рефакторинг в `internal/geocoder`: модели `Country`, `City`, `POI`, `Region`, `search.go` с package declaration и репозитории без реализации.

Фронтенд и реализованные репозитории в текущем репозитории отсутствуют.

---

## Исправлено по комментариям

- `database.Connect()` теперь является единственным местом подключения к БД.
- Локальный DSN синхронизирован с `docker-compose.yml`: `postgres://geo:geopass@localhost:5432/geocoder?sslmode=disable`.
- `DATABASE_URL` остаётся переопределением для Railway или другой среды.
- Неиспользуемый `apiKeyMiddleware()` и всё упоминание `API_KEY` удалены.
- Добавлена миграция `migrations/001_places.sql`.
- Добавлена миграция `migrations/006_users.sql`.
- Добавлена модель `internal/geocoder/region.go`.
- `CREATE EXTENSION IF NOT EXISTS postgis;` добавлен в миграции.
- HTML-комментарии из документации удалены.

---

## Стек

| Компонент           | Технология                           |
| ------------------- | ------------------------------------ |
| Язык                | Go 1.25.0                            |
| HTTP-сервер         | Gin                                  |
| PostgreSQL-драйвер  | `github.com/lib/pq`                  |
| JWT                 | `github.com/golang-jwt/jwt/v5`       |
| Хеширование паролей | `golang.org/x/crypto/bcrypt`         |
| БД                  | PostgreSQL 16 + PostGIS 3.4 локально |
| Локальная БД        | Docker Compose                       |
| Миграции            | SQL-файлы в `migrations/`            |

---

## Структура проекта

```text
geogo/
├── docker-compose.yml
├── go.mod
├── go.sum
├── main.go
├── docs/
│   ├── PROJECT_STRUCTURE.txt
│   └── geocoder-session.md
├── internal/
│   ├── database/
│   │   └── postgres.go
│   └── geocoder/
│       ├── country.go
│       ├── city.go
│       ├── poi.go
│       ├── region.go
│       ├── search.go
│       └── repository/
│           ├── country_repository.go
│           ├── city_repository.go
│           └── poi_repository.go
└── migrations/
    ├── 001_places.sql
    ├── 002_countries.sql
    ├── 003_cities.sql
    ├── 004_pois.sql
    ├── 005_regions.sql
    └── 006_users.sql
```

---

## docker-compose.yml

Локальная БД запускается через Docker Compose:

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

## internal/database/postgres.go

Пакет `database` содержит единственную функцию подключения к PostgreSQL:

```go
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
```

Порядок выбора DSN:

1. `DATABASE_URL`, если переменная задана.
2. Локальный DSN из `defaultDSN`.

`database.Connect()` открывает подключение и сразу выполняет `Ping()`.

---

## main.go

`main.go` — текущая точка входа и монолитный HTTP-сервер.

### Импорты

```go
database/sql
log
net/http
os
strconv
time

geogo/internal/database

github.com/gin-gonic/gin
github.com/golang-jwt/jwt/v5
_ "github.com/lib/pq"
golang.org/x/crypto/bcrypt
```

### Подключение к БД

```go
db, err = database.Connect()
if err != nil {
	log.Fatal("Ошибка подключения к БД:", err)
}
defer db.Close()
```

`main.go` больше не создаёт подключение напрямую через `sql.Open()`.

### Структуры

```go
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
```

### Middleware

| Middleware          | Статус     | Описание                                  |
| ------------------- | ---------- | ----------------------------------------- |
| `adminMiddleware()` | Используется | Проверяет роль `admin` из JWT-claims      |
| `authMiddleware()`  | Используется | Проверяет `Authorization: Bearer <token>` |

Также в `main()` подключён CORS middleware:

```go
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization
```

### Роуты

```go
r.POST("/auth/register", register)
r.POST("/auth/login", login)
r.GET("/geocode", geocode)
r.GET("/reverse", reverseGeocode)
r.GET("/places", getPlaces)

protected := r.Group("/", authMiddleware())
protected.GET("/me", getMe)

admin := r.Group("/", authMiddleware(), adminMiddleware())
admin.POST("/places", addPlace)
admin.DELETE("/places/:id", deletePlace)
```

---

## Эндпоинты API

| Метод    | URL                        | Авторизация   | Описание                              |
| -------- | -------------------------- | ------------- | ------------------------------------- |
| `POST`   | `/auth/register`           | Нет           | Регистрация пользователя              |
| `POST`   | `/auth/login`              | Нет           | Вход, возвращает JWT                  |
| `GET`    | `/me`                      | JWT           | Данные текущего пользователя          |
| `GET`    | `/geocode?address=...`     | Нет           | Поиск места по адресу/названию/городу |
| `GET`    | `/reverse?lat=...&lon=...` | Нет           | Поиск ближайшего места по координатам |
| `GET`    | `/places`                  | Нет           | Список всех мест                      |
| `POST`   | `/places`                  | JWT + `admin` | Добавить место                        |
| `DELETE` | `/places/:id`              | JWT + `admin` | Удалить место                         |

---

## Геокодинг и reverse-геокодинг

### `GET /geocode?address=...`

Ищет одно ближайшее по `LIMIT 1` место в таблице `places`:

```sql
SELECT id, name, country, city, address,
       ST_Y(location::geometry) AS lat,
       ST_X(location::geometry) AS lon
FROM places
WHERE address ILIKE $1 OR name ILIKE $1 OR city ILIKE $1
LIMIT 1
```

Если ничего не найдено:

```json
{
  "error": "Место не найдено"
}
```

### `GET /reverse?lat=...&lon=...`

Ищет ближайшее место через PostGIS KNN-оператор `<->`:

```sql
SELECT id, name, country, city, address,
       ST_Y(location::geometry) AS lat,
       ST_X(location::geometry) AS lon
FROM places
ORDER BY location <-> ST_SetSRID(ST_MakePoint($1, $2), 4326)
LIMIT 1
```

Параметры передаются в порядке PostGIS: `$1 = lon`, `$2 = lat`.

---

## CRUD мест

### `POST /places`

Требует JWT с ролью `admin`.

Принимает JSON:

```json
{
  "name": "Кремль",
  "country": "Russia",
  "city": "Moscow",
  "address": "Красная площадь, 1",
  "lat": 55.7520,
  "lon": 37.6175
}
```

Сохраняет точку через PostGIS:

```sql
INSERT INTO places (name, country, city, address, location)
VALUES ($1, $2, $3, $4, ST_SetSRID(ST_MakePoint($5, $6), 4326))
```

### `GET /places`

Возвращает список мест из `places` с координатами через `ST_Y` и `ST_X`.

### `DELETE /places/:id`

Требует JWT с ролью `admin`.

```sql
DELETE FROM places WHERE id = $1
```

---

## JWT-авторизация

JWT реализован в `main.go`.

### Регистрация

```go
POST /auth/register
```

Пароль хэшируется через `bcrypt.GenerateFromPassword`.

Таблица `users` теперь описана в `migrations/006_users.sql`.

### Логин

```go
POST /auth/login
```

Проверяет email/password и возвращает:

```json
{
  "token": "<jwt>",
  "email": "user@example.com",
  "role": "admin"
}
```

### Токен

Claims:

```go
type Claims struct {
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}
```

JWT:

- алгоритм: `HS256`
- срок жизни: 24 часа
- secret берётся из `JWT_SECRET`

### `/me`

Возвращает данные из JWT:

```json
{
  "id": 1,
  "email": "user@example.com",
  "role": "admin"
}
```

---

## Модели в `internal/geocoder`

### `internal/geocoder/country.go`

```go
type Country struct {
	ID      int64
	Name    string
	ISOCode string
	Lat     float64
	Lon     float64
}
```

### `internal/geocoder/city.go`

```go
type City struct {
	ID        int64
	CountryID int64
	Name      string
	Lat       float64
	Lon       float64
}
```

### `internal/geocoder/poi.go`

```go
type POI struct {
	ID      int64
	CityID  int64
	Name    string
	Address string
	Lat     float64
	Lon     float64
}
```

### `internal/geocoder/region.go`

```go
type Region struct {
	ID        int64
	CountryID int64
	Name      string
	Lat       float64
	Lon       float64
}
```

Модели пока не имеют JSON-тегов и поля `Location`.

---

## Репозитории в `internal/geocoder/repository`

Файлы существуют, но пока без реализации:

| Файл                    | Статус    |
| ----------------------- | --------- |
| `country_repository.go` | Заготовка |
| `city_repository.go`    | Заготовка |
| `poi_repository.go`     | Заготовка |

Планируемое назначение:

- `CountryRepository` — операции над `countries`
- `CityRepository` — операции над `cities`
- `POIRepository` — операции над `pois`

---

## Миграции

### `migrations/001_places.sql`

Создаёт PostGIS extension, таблицу `places` и GIST-индекс:

```sql
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
```

### `migrations/002_countries.sql`

```sql
CREATE TABLE IF NOT EXISTS countries (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    iso_code CHAR(2) UNIQUE,

    lat DOUBLE PRECISION,
    lon DOUBLE PRECISION,

    location GEOMETRY(Point, 4326)
);

CREATE INDEX countries_location_idx
ON countries USING GIST(location);
```

### `migrations/003_cities.sql`

```sql
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
```

### `migrations/004_pois.sql`

```sql
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
```

### `migrations/005_regions.sql`

```sql
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
```

### `migrations/006_users.sql`

```sql
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,

    email    TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL,
    role     TEXT NOT NULL DEFAULT 'user',

    created_at TIMESTAMP DEFAULT NOW()
);
```

Во всех миграциях с PostGIS-геометрией используется `GEOMETRY(Point, 4326)` и GIST-индекс по `location`.

---

## go.mod

```go
module geogo

go 1.25.0
```

Основные зависимости:

| Dependency                     | Назначение         |
| ------------------------------ | ------------------ |
| `github.com/gin-gonic/gin`     | HTTP-сервер        |
| `github.com/lib/pq`            | PostgreSQL-драйвер |
| `github.com/golang-jwt/jwt/v5` | JWT                |
| `golang.org/x/crypto`          | bcrypt             |

---

## Переменные окружения

| Переменная     | Используется | Где                                                      |
| -------------- | ------------ | -------------------------------------------------------- |
| `DATABASE_URL` | Да           | `internal/database/postgres.go`, опциональное переопределение DSN |
| `JWT_SECRET`   | Да           | `login()` и `authMiddleware()`                           |

`DATABASE_URL` не обязателен для локального запуска: если переменная не задана, используется DSN из `docker-compose.yml`.

---

## Запуск локально

```bash
docker compose up -d
go run .
```

Перед первым запуском нужно применить миграции:

```bash
psql postgres://geo:geopass@localhost:5432/geocoder -f migrations/001_places.sql
psql postgres://geo:geopass@localhost:5432/geocoder -f migrations/002_countries.sql
psql postgres://geo:geopass@localhost:5432/geocoder -f migrations/003_cities.sql
psql postgres://geo:geopass@localhost:5432/geocoder -f migrations/004_pois.sql
psql postgres://geo:geopass@localhost:5432/geocoder -f migrations/005_regions.sql
psql postgres://geo:geopass@localhost:5432/geocoder -f migrations/006_users.sql
```

Для Railway или другой среды достаточно задать:

```bash
DATABASE_URL=<строка подключения к PostgreSQL>
JWT_SECRET=<секретный ключ>
```

---

## Примеры запросов

### Регистрация

```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"secret","role":"admin"}'
```

### Логин

```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"secret"}'
```

Ответ:

```json
{
  "token": "<jwt>",
  "email": "admin@example.com",
  "role": "admin"
}
```

### Данные текущего пользователя

```bash
curl http://localhost:8080/me \
  -H "Authorization: Bearer <jwt>"
```

### Список мест

```bash
curl http://localhost:8080/places
```

### Добавить место

```bash
curl -X POST http://localhost:8080/places \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt>" \
  -d '{
    "name": "Кремль",
    "country": "Russia",
    "city": "Moscow",
    "address": "Красная площадь, 1",
    "lat": 55.7520,
    "lon": 37.6175
  }'
```

### Геокодинг

```bash
curl "http://localhost:8080/geocode?address=Кремль"
```

### Reverse-геокодинг

```bash
curl "http://localhost:8080/reverse?lat=55.75&lon=37.61"
```

### Удалить место

```bash
curl -X DELETE http://localhost:8080/places/1 \
  -H "Authorization: Bearer <jwt>"
```

---

## Vue-фронтенд

В текущем репозитории нет файлов фронтенда.

Старые разделы про Vue 3, Leaflet, Axios, вкладки, карту и деплой фронта относятся к предыдущему плану, но не соответствуют текущей структуре проекта.

---

## Что можно улучшить

- Реализовать репозитории в `internal/geocoder/repository`
- Реализовать сервис поиска в `internal/geocoder/search.go`
- Перенести handlers из `main.go` в отдельный слой
- Использовать модели `countries`, `cities`, `pois`, `regions` в геокодере
- Добавить JSON-теги моделям `Country`, `City`, `POI`, `Region`
- Добавить `Location`/PostGIS-поля в модели, если они нужны наружу
- Добавить тесты для auth, geocode, reverse-geocode и CRUD мест

---

## Итог

Проект находится на переходной стадии:

- рабочий API уже есть в `main.go`;
- JWT-авторизация и админские операции реализованы;
- геокодинг и reverse-геокодинг работают через таблицу `places`;
- подключение к БД вынесено в `internal/database/postgres.go`;
- базовые миграции для `places`, `users`, `countries`, `cities`, `pois` и `regions` добавлены;
- рефакторинг на `internal/geocoder` начат, но модели пока не связаны с БД;
- репозитории и сервис поиска требуют реализации.
