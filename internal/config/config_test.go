package config

import (
	"os"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{"mock provider no key", &Config{Provider: "mock", OpenAIKey: ""}, false},
		{"openai no key fails", &Config{Provider: "openai", OpenAIKey: ""}, true},
		{"anthropic no key fails", &Config{Provider: "anthropic", AnthropicKey: ""}, true},
		{"valid port", &Config{Provider: "mock", Port: "8080"}, false},
		{"invalid port", &Config{Provider: "mock", Port: "abc"}, true},
		{"valid language ru", &Config{Provider: "mock", Language: "ru"}, false},
		{"valid language en", &Config{Provider: "mock", Language: "en"}, false},
		{"valid language zh", &Config{Provider: "mock", Language: "zh"}, false},
		{"invalid language", &Config{Provider: "mock", Language: "de"}, true},
		{"empty language defaults ok", &Config{Provider: "mock", Language: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_KEY", "test_value")
	defer os.Unsetenv("TEST_KEY")

	tests := []struct {
		name      string
		key       string
		defaultVal string
		want     string
	}{
		{"existing key", "TEST_KEY", "default", "test_value"},
		{"missing key", "MISSING_KEY", "default", "default"},
		{"empty value uses default", "TEST_KEY", "default", "test_value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getEnv(tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Run("loads with defaults", func(t *testing.T) {
		os.Unsetenv("PROVIDER")
		os.Unsetenv("LLM")
		os.Unsetenv("API_KEY")
		os.Unsetenv("PORT")
		os.Unsetenv("LANGUAGE")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}

		if cfg.Provider != "mock" {
			t.Errorf("Provider = %q, want %q", cfg.Provider, "mock")
		}
		if cfg.LLM != "gpt-4o" {
			t.Errorf("LLM = %q, want %q", cfg.LLM, "gpt-4o")
		}
		if cfg.Port != "8080" {
			t.Errorf("Port = %q, want %q", cfg.Port, "8080")
		}
		if cfg.Language != "ru" {
			t.Errorf("Language = %q, want %q", cfg.Language, "ru")
		}
	})

	t.Run("loads from env", func(t *testing.T) {
		os.Setenv("PROVIDER", "openai")
		os.Setenv("LLM", "gpt-4o-mini")
		os.Setenv("OPENAI_API_KEY", "sk-test123")
		os.Setenv("PORT", "3000")
		os.Setenv("LANGUAGE", "en")
		defer func() {
			os.Unsetenv("PROVIDER")
			os.Unsetenv("LLM")
			os.Unsetenv("OPENAI_API_KEY")
			os.Unsetenv("ANTHROPIC_API_KEY")
			os.Unsetenv("PORT")
			os.Unsetenv("LANGUAGE")
		}()

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}

		if cfg.Provider != "openai" {
			t.Errorf("Provider = %q, want %q", cfg.Provider, "openai")
		}
		if cfg.LLM != "gpt-4o-mini" {
			t.Errorf("LLM = %q, want %q", cfg.LLM, "gpt-4o-mini")
		}
		if cfg.OpenAIKey != "sk-test123" {
			t.Errorf("OpenAIKey = %q, want %q", cfg.OpenAIKey, "sk-test123")
		}
		if cfg.Port != "3000" {
			t.Errorf("Port = %q, want %q", cfg.Port, "3000")
		}
		if cfg.Language != "en" {
			t.Errorf("Language = %q, want %q", cfg.Language, "en")
		}
	})
}