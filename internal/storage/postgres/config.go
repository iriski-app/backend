package postgres

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBUser     string
	DBPassword string
	DBName     string
	DBHost     string
	DBPort     string
}

func GetenvWithDefault(cuery, defaultValue string) string {
	if value, exists := os.LookupEnv(cuery); exists {
		return value
	} else {
		return defaultValue
	}
}

func LoadConfig() (Config, error) {
	err := godotenv.Overload()
	if err != nil {
		return Config{}, err
	}

	dbUser := GetenvWithDefault("DB_USER", "postgres")
	dbPassword := GetenvWithDefault("DB_PASSWORD", "postgres")
	dbName := GetenvWithDefault("POSTGRES_DB", "risk_metrics")
	dbHost := GetenvWithDefault("DB_HOST", "localhost")
	dbPort := GetenvWithDefault("DB_PORT", "5432")

	return Config{
		DBUser:     dbUser,
		DBPassword: dbPassword,
		DBName:     dbName,
		DBHost:     dbHost,
		DBPort:     dbPort,
	}, nil
}

func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		c.DBHost,
		c.DBUser,
		c.DBPassword,
		c.DBName,
		c.DBPort,
	)
}
