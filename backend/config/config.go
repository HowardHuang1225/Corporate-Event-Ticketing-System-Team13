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
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Try to assemble from individual components
		dbHost := getEnv("DB_HOST", "localhost")
		dbUser := getEnv("DB_USER", "ts_user")
		dbPass := getEnv("DB_PASSWORD", "ts_password")
		dbName := getEnv("DB_NAME", "ticketing_system")
		dbPort := getEnv("DB_PORT", "5432")
		dbURL = "host=" + dbHost + " user=" + dbUser + " password=" + dbPass + " dbname=" + dbName + " port=" + dbPort + " sslmode=disable"
	}

	return &Config{
		DatabaseURL:    dbURL,
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379"),
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
