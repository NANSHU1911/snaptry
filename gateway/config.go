package main

import "os"

// Config is loaded entirely from environment variables. No secrets are
// hardcoded anywhere in this project.
type Config struct {
	Port             string
	GatewayAPIKey    string // X-API-Key expected from the extension
	ProviderName     string // "falai" (default) or "fitroom"
	FalAIKey         string // FALAI_API_KEY
	FitroomKey       string // FITROOM_API_KEY
	FitroomEndpoint  string // FITROOM_ENDPOINT (optional override)
	RateLimitPerHour int
	CacheTTLHours    int
	MaxImageBytes    int64
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadConfig() Config {
	return Config{
		Port:             getenv("PORT", "8080"),
		GatewayAPIKey:    os.Getenv("GATEWAY_API_KEY"),
		ProviderName:     getenv("PROVIDER", "fitroom"),
		FalAIKey:         os.Getenv("FALAI_API_KEY"),
		FitroomKey:       os.Getenv("FITROOM_API_KEY"),
		FitroomEndpoint:  os.Getenv("FITROOM_ENDPOINT"),
		RateLimitPerHour: 20,
		CacheTTLHours:    24 * 7,
		MaxImageBytes:    8 << 20, // 8 MiB per image (decoded)
	}
}
