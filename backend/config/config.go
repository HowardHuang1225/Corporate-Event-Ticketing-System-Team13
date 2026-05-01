package config

import "os"

type Config struct {
	DatabaseURL string
	RedisURL    string
	JWTSecret      string
	Port           string
	AllowedOrigins string
}

func Load() *Config {
	return &Config{
		DatabaseURL: getEnv("DATABASE_URL", "host=localhost user=ts_user password=ts_password dbname=ticketing_system port=5432 sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:      getEnv("JWT_SECRET", "dev-jwt-secret-change-in-prod-32chars!!"),
		Port:           getEnv("PORT", "8001"),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000,http://localhost:8080"),
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
