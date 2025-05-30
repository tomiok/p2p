package config

import (
	"os"
)

type Config struct {
	Port          string
	LiveKitURL    string
	LiveKitKey    string
	LiveKitSecret string
	Environment   string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		LiveKitURL:    getEnv("LIVEKIT_URL", "ws://localhost:7880"),
		LiveKitKey:    getEnv("LIVEKIT_API_KEY", "devkey"),
		LiveKitSecret: getEnv("LIVEKIT_API_SECRET", "secret"),
		Environment:   getEnv("ENVIRONMENT", "development"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
