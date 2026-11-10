package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"

	"example.com/app/internal/pkg/response"
)

const headerRequestID = "X-Request-ID"

// RequestID 透传上游请求 ID，没有则生成一个，便于日志串联。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(headerRequestID)
		if id == "" {
			id = newRequestID()
		}
		c.Set(response.RequestIDKey, id)
		c.Header(headerRequestID, id)
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
