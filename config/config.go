package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GINMode       string
	DBDriver      string
	DBSource      string
	ServerAddress string
}

func LoadConfig() (config Config, err error) {
	err = godotenv.Load()

	config = Config{
		GINMode:       getEnv("GIN_MODE", "debug"),
		DBDriver:      getEnv("DB_DRIVER", "postgres"),
		DBSource:      getEnv("DB_SOURCE", ""),
		ServerAddress: getEnv("SERVER_ADDRESS", "0.0.0.0:8080"),
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
