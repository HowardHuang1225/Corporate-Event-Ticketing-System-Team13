package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	RedisURL    string
	JWTSecret      string
	Port           string
	AllowedOrigins string
}

func Load() *Config {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbHost := getEnv("DB_HOST", "localhost")
		dbUser := getEnv("DB_USER", "ts_user")
		dbPass := getEnv("DB_PASSWORD", "ts_password")
		dbName := getEnv("DB_NAME", "ticketing_system")
		dbPort := getEnv("DB_PORT", "5432")
		dbURL = "host=" + dbHost + " user=" + dbUser + " password=" + dbPass + " dbname=" + dbName + " port=" + dbPort + " sslmode=disable"
	}

	return &Config{
		DatabaseURL:       dbURL,
		DBMaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 200),
		DBMaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 50),
		DBConnMaxLifetime: time.Duration(getEnvAsInt("DB_CONN_MAX_LIFETIME_MINUTES", 30)) * time.Minute,
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:         getEnv("JWT_SECRET", "dev-jwt-secret-change-in-prod-32chars!!"),
		Port:              getEnv("PORT", "8001"),
		AllowedOrigins:    getEnv("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000,http://localhost:8080"),
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if val, err := strconv.Atoi(v); err == nil {
			return val
		}
	}
	return defaultVal
}