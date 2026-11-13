// Package webui serves the built SPA without intercepting API or health errors.
package webui

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	web "geo-assist/internal/platform/httpserver"
	app "geo-assist/internal/shared/application"
	"github.com/gin-gonic/gin"
)

func Handler(root string) gin.HandlerFunc {
	return func(c *gin.Context) {
		missing := func() { web.Fail(c, app.E(404, "NOT_FOUND", "Route not found")) }
		p := c.Request.URL.Path
		if root == "" || (c.Request.Method != "GET" && c.Request.Method != "HEAD") || p == "/api" || strings.HasPrefix(p, "/api/") || p == "/health" || strings.HasPrefix(p, "/health/") {
			missing()
			return
		}
		for _, segment := range strings.Split(strings.ReplaceAll(p, "\\", "/"), "/") {
			if strings.HasPrefix(segment, ".") {
				missing()
				return
			}
		}
		requested := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path.Clean(p), "/")))
		info, err := os.Stat(requested)
		asset := strings.HasPrefix(p, "/assets/")
		if err != nil || info.IsDir() {
			if asset || path.Ext(p) != "" {
				missing()
				return
			}
			requested = filepath.Join(root, "index.html")
		}
		resolved, err := filepath.EvalSymlinks(requested)
		base, baseErr := filepath.EvalSymlinks(root)
		rel, relErr := filepath.Rel(base, resolved)
		if err != nil || baseErr != nil || relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			missing()
			return
		}
		f, err := os.Open(resolved)
		if err != nil {
			missing()
			return
		}
		defer f.Close()
		info, err = f.Stat()
		if err != nil || info.IsDir() {
			missing()
			return
		}
		if asset {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			c.Header("Cache-Control", "no-cache")
		}
		c.Status(http.StatusOK)
		http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), f)
	}
}
