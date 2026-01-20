package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// HTTPMetrics is Gin middleware that records HTTP request metrics.
// It tracks request count and latency by method, path, and status code.
func HTTPMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath() // Use route pattern, not actual path (avoids cardinality explosion)
		if path == "" {
			path = "unknown" // For NoRoute handler
		}
		method := c.Request.Method

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}
