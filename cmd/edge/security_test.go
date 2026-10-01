package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/cdn-simulator/internal/config"
)

func newTestServer(apiKey string) *EdgeServer {
	return &EdgeServer{
		config: &config.Config{
			Edge:   config.EdgeConfig{AdminAPIKey: apiKey},
			Origin: config.OriginConfig{Backends: []string{"http://origin:9090"}},
		},
	}
}

func TestOriginURLBlocksSSRF(t *testing.T) {
	s := newTestServer("")

	blocked := []string{
		"@169.254.169.254/latest/meta-data/",
		"//evil.com/steal",
		"http://evil.com/steal",
		"https://evil.com/steal",
		"api/v1/relative",
		"/ok@evil.com",
	}
	for _, p := range blocked {
		if got, err := s.originURL(p); err == nil {
			t.Errorf("originURL(%q) = %q, want error", p, got)
		}
	}

	allowed := []string{"/api/v1/assets/logo.png", "/health"}
	for _, p := range allowed {
		got, err := s.originURL(p)
		if err != nil {
			t.Errorf("originURL(%q) unexpected error: %v", p, err)
			continue
		}
		if !strings.HasPrefix(got, "http://origin:9090/") {
			t.Errorf("originURL(%q) = %q, want origin prefix", p, got)
		}
	}
}

func testAdminRoute(t *testing.T, s *EdgeServer, apiKeyHeader string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/purge", s.requireAPIKey(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/purge", strings.NewReader("{}"))
	if apiKeyHeader != "" {
		req.Header.Set("X-API-Key", apiKeyHeader)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestAdminAPIRejectsWithoutKey(t *testing.T) {
	if w := testAdminRoute(t, newTestServer("secret"), ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no key: status = %d, want 401", w.Code)
	}
	if w := testAdminRoute(t, newTestServer(""), "anything"); w.Code != http.StatusServiceUnavailable {
		t.Errorf("unset config: status = %d, want 503", w.Code)
	}
	if w := testAdminRoute(t, newTestServer("secret"), "wrong-key"); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong key: status = %d, want 401", w.Code)
	}
}

func TestAdminAPIAcceptsValidKey(t *testing.T) {
	if w := testAdminRoute(t, newTestServer("secret"), "secret"); w.Code != http.StatusOK {
		t.Errorf("valid key: status = %d, want 200", w.Code)
	}
}
