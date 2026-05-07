
import {

}


func openTestDB(t *testing.T, target string, DSN_param []any) (*gorm.DB, error) {
	t.Helper()

	db, err := gorm.Open(postgres.Open(testDSN(dbName)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("Postgres is not available for tests: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to access sql db: %w", err)
	}
	t.Cleanup(func() {
		sqlDB.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("Postgres is not reachable for tests: %w", err)
	}

	if err := db.AutoMigrate(DSN_param...); err != nil {
		return nil, fmt.Errorf("failed to migrate test tables: %w", err)
	}

	return db, nil
}

func testDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}

	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		envOrDefault("DB_HOST", "localhost"),
		envOrDefault("DB_USER", "ts_user"),
		envOrDefault("DB_PASSWORD", "ts_password"),
		envOrDefault("DB_NAME", "ticketing_system"),
		envOrDefault("DB_PORT", "5432"),
	)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func seedTestRole(db *gorm.DB, role any) (*model.User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, fmt.Errorf("failed to hash manager password: %w", err)
	}

	if err := db.Create(&role).Error; err != nil {
		return any, fmt.Errorf("failed to seed role: %w", err)
	}
	return role, nil
}