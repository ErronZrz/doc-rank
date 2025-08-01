package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	RedisAddr string
	RedisDB   int
	Port      string
}

func Load() Config {
	// 尝试加载 .env 文件（默认从当前目录查找）
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or load failed, fallback to environment variables")
	}

	return Config{
		RedisAddr: getEnv("REDIS_ADDR", "localhost:6379"),
		RedisDB:   getEnvAsInt("REDIS_DB", 0),
		Port:      getEnv("PORT", "8080"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvAsInt(name string, defaultVal int) int {
	if valStr := getEnv(name, ""); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil {
			return val
		}
	}
	return defaultVal
}
