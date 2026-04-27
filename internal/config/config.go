package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Provider  string
	LLM       string
	OpenAIKey string
	AnthropicKey string
	Endpoint  string
	Project   string
	Port      string
	Language  string
}

func (c *Config) Validate() error {
	if c.Provider == "openai" && c.OpenAIKey == "" {
		return errors.New("OPENAI_API_KEY required for openai provider")
	}
	if c.Provider == "anthropic" && c.AnthropicKey == "" {
		return errors.New("ANTHROPIC_API_KEY required for anthropic provider")
	}
	if c.Port != "" {
		if _, err := strconv.Atoi(c.Port); err != nil {
			return fmt.Errorf("invalid PORT: %w", err)
		}
	}
	if c.Language != "" && c.Language != "ru" && c.Language != "en" && c.Language != "zh" {
		return errors.New("LANGUAGE must be 'ru', 'en' or 'zh'")
	}
	return nil
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading .env: %w", err)
	}

	cfg := &Config{
		Provider:  getEnv("PROVIDER", "mock"),
		LLM:      getEnv("LLM", "gpt-4o"),
		OpenAIKey:   os.Getenv("OPENAI_API_KEY"),
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		Endpoint: os.Getenv("ENDPOINT"),
		Project:  os.Getenv("PROJECT"),
		Port:     getEnv("PORT", "8080"),
		Language: getEnv("LANGUAGE", "ru"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}