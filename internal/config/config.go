package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Provider    string
	Model       string
	BaseURL     string
	APIKey      string
	Timeout     time.Duration
	Temperature float64
	Milvus      MilvusConfig
}

type MilvusConfig struct {
	Enabled    bool
	Address    string
	Token      string
	Collection string
	Dim        int
	Timeout    time.Duration
}

func Load() Config {
	loadDotEnv(".env")
	timeoutSeconds := intEnv("LLM_TIMEOUT_SECONDS", 45)
	return Config{
		Provider:    env("LLM_PROVIDER", "demo"),
		Model:       env("LLM_MODEL", "demo-mediaflow"),
		BaseURL:     env("LLM_BASE_URL", "https://api.openai.com/v1"),
		APIKey:      os.Getenv("LLM_API_KEY"),
		Timeout:     time.Duration(timeoutSeconds) * time.Second,
		Temperature: floatEnv("LLM_TEMPERATURE", 0.3),
		Milvus: MilvusConfig{
			Enabled:    boolEnv("MILVUS_ENABLED", false),
			Address:    env("MILVUS_ADDRESS", "http://localhost:19530"),
			Token:      env("MILVUS_TOKEN", "root:Milvus"),
			Collection: env("MILVUS_COLLECTION", "mediaflow_knowledge"),
			Dim:        intEnv("EMBEDDING_DIM", 64),
			Timeout:    time.Duration(intEnv("MILVUS_TIMEOUT_SECONDS", 15)) * time.Second,
		},
	}
}

func loadDotEnv(path string) {
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func intEnv(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func floatEnv(key string, fallback float64) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}

func boolEnv(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
