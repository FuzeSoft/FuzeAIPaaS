package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

var skipPaths = map[string]struct{}{
	"/metrics":          {},
	"/api/v1/health":    {},
}

func NewMiddleware(r *Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, skip := skipPaths[c.Request.URL.Path]; skip {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		elapsed := time.Since(start).Seconds()

		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		r.ObserveRequest(c.Request.Method, path, status, elapsed)
	}
}