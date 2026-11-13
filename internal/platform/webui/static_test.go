package webui

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSPAFallbackAndReservedPaths(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><title>GeoAssist</title>"), 0600)
	os.Mkdir(filepath.Join(root, "assets"), 0700)
	os.WriteFile(filepath.Join(root, "assets", "app-abc.js"), []byte("console.log('ok')"), 0600)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.NoRoute(Handler(root))
	for _, tt := range []struct {
		method, path string
		status       int
		html         bool
	}{
		{"GET", "/", 200, true}, {"GET", "/places/123", 200, true}, {"HEAD", "/profile", 200, false},
		{"GET", "/assets/app-abc.js", 200, false}, {"GET", "/assets/missing.js", 404, false},
		{"GET", "/api/v1/missing", 404, false}, {"GET", "/api", 404, false}, {"GET", "/health/missing", 404, false},
		{"GET", "/.env", 404, false}, {"GET", "/../secret", 404, false}, {"POST", "/profile", 404, false},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
		if w.Code != tt.status {
			t.Errorf("%s %s: %d", tt.method, tt.path, w.Code)
		}
		if strings.Contains(w.Body.String(), "<!doctype") != tt.html {
			t.Errorf("unexpected HTML: %s", tt.path)
		}
	}
}
