package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Provider  string
	LLM       string
	APIKey    string
	Endpoint  string
	Project   string
	Port      string
}

func Load() (*Config, error) {
	godotenv.Load()

	cfg := &Config{
		Provider: getEnv("PROVIDER", "mock"),
		LLM:      getEnv("LLM", "gpt-4o"),
		APIKey:   os.Getenv("API_KEY"),
		Endpoint: os.Getenv("ENDPOINT"),
		Project:  os.Getenv("PROJECT"),
		Port:     getEnv("PORT", "8080"),
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}