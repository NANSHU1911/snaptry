// SnapTry try-on gateway (MVP).
//
// POST /api/tryon      submit a job   (X-API-Key required)
// GET  /api/tryon/{id} poll a job     (X-API-Key required)
// GET  /health            liveness     (open)
//
// All configuration comes from environment variables; see config.go.
// No secrets are hardcoded.
package main

import (
	"log"
	"net/http"
	"time"

	"snaptry-gateway/provider"
)

func main() {
	cfg := loadConfig()

	var prov provider.Provider
	switch cfg.ProviderName {
	case "fitroom":
		prov = provider.NewFitroom(cfg.FitroomKey, cfg.FitroomEndpoint)
	case "falai":
		prov = provider.NewFalAI(cfg.FalAIKey)
	default:
		log.Fatalf("unknown PROVIDER %q (want falai|fitroom)", cfg.ProviderName)
	}

	cache := NewCache(cfg.CacheTTLHours)
	store := NewStore(prov, cache)
	srv := &Server{
		cfg:     cfg,
		store:   store,
		limiter: NewRateLimiter(cfg.RateLimitPerHour, time.Hour),
	}

	addr := ":" + cfg.Port
	log.Printf("snaptry-gateway listening on %s provider=%s", addr, prov.Name())
	if cfg.GatewayAPIKey == "" {
		log.Printf("WARNING: GATEWAY_API_KEY is empty — /api/* will return 503 until it is set")
	}
	if err := http.ListenAndServe(addr, srv.logRequest(srv.routes())); err != nil {
		log.Fatal(err)
	}
}
