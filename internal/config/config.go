package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr    string
	Hostname      string
	AdminPassword string
	DBPath        string
	APIToken      string
}

func Load() *Config {
	return &Config{
		ListenAddr:    getEnv("LISTEN", ":8090"),
		Hostname:      getEnv("HOSTNAME", "http://localhost:8090"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "12345678"),
		DBPath:        getEnv("DB_PATH", "data/mimo.db"),
		APIToken:      getEnv("API_TOKEN", ""),
	}
}

func (c *Config) GetBaseURL() string {
	return strings.TrimRight(c.Hostname, "/")
}

func (c *Config) HasAPIToken() bool {
	return c.APIToken != ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
