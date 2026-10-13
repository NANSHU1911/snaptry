package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"snaptry-gateway/provider"
)

type Server struct {
	cfg     Config
	store   *Store
	limiter *RateLimiter
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// requireAuth guards /api/*; /health stays open.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.GatewayAPIKey == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway not configured (GATEWAY_API_KEY missing)"})
			return
		}
		if r.Header.Get("X-API-Key") != s.cfg.GatewayAPIKey {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"ok":       "true",
		"provider": s.store.prov.Name(),
		"version":  "0.1.0",
	})
}

type tryonBody struct {
	PersonImage  string `json:"person_image"`
	GarmentImage string `json:"garment_image"`
	Category     string `json:"category"`
}

// imageBytes decodes a data: URI (or returns raw bytes for http URLs, which
// are passed through untouched) so we can enforce a size cap.
func imageBytes(s string, maxBytes int64) (int64, error) {
	if strings.HasPrefix(s, "data:") {
		comma := strings.Index(s, ",")
		if comma < 0 {
			return 0, errBadImage("malformed data URI")
		}
		raw, err := base64.StdEncoding.DecodeString(s[comma+1:])
		if err != nil {
			// try URL-safe alphabet as a fallback
			raw, err = base64.URLEncoding.DecodeString(s[comma+1:])
			if err != nil {
				return 0, errBadImage("base64 decode failed")
			}
		}
		if int64(len(raw)) > maxBytes {
			return 0, errBadImage("image too large")
		}
		return int64(len(raw)), nil
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return 0, nil // remote URL: provider fetches it; no local size check
	}
	return 0, errBadImage("must be data: URI or http(s) URL")
}

type badImageError struct{ msg string }

func (e badImageError) Error() string { return e.msg }
func errBadImage(m string) error      { return badImageError{m} }

func (s *Server) handleTryonSubmit(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.Allow(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded, try later"})
		return
	}
	var b tryonBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&b); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if b.PersonImage == "" || b.GarmentImage == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "person_image and garment_image are required"})
		return
	}
	if _, err := imageBytes(b.PersonImage, s.cfg.MaxImageBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "person_image: " + err.Error()})
		return
	}
	if _, err := imageBytes(b.GarmentImage, s.cfg.MaxImageBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "garment_image: " + err.Error()})
		return
	}
	category := b.Category
	if category == "" {
		category = "auto"
	}
	t := s.store.Submit(provider.TryOnRequest{
		PersonImage:  b.PersonImage,
		GarmentImage: b.GarmentImage,
		Category:     category,
	})
	log.Printf("POST /api/tryon ip=%s task=%s status=%s", ip, t.ID, t.Status)
	writeJSON(w, http.StatusAccepted, map[string]string{"task_id": t.ID, "status": t.Status})
}

func (s *Server) handleTryonGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tryon/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad task id"})
		return
	}
	t, ok := s.store.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/tryon", s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		s.handleTryonSubmit(w, r)
	}))
	mux.HandleFunc("/api/tryon/", s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		s.handleTryonGet(w, r)
	}))
	return mux
}

func (s *Server) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s ip=%s %v", r.Method, r.URL.Path, clientIP(r), time.Since(start).Round(time.Millisecond))
	})
}
