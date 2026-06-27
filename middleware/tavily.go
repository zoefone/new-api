package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func APIKeyTokenAuthCompat() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			if key := strings.TrimSpace(c.GetHeader("X-API-Key")); key != "" {
				c.Request.Header.Set("Authorization", "Bearer "+key)
			}
		}
		c.Next()
	}
}

func TavilyTokenAuthCompat() gin.HandlerFunc {
	return APIKeyTokenAuthCompat()
}
