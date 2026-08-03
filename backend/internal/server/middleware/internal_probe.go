package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/internalprobe"
	"github.com/gin-gonic/gin"
)

// InternalProbe converts a valid signed wire marker into a private request
// context value. Invalid markers are stripped and otherwise ignored.
func InternalProbe(auth *internalprobe.Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil && c.Request.Header.Get(internalprobe.HeaderName) != "" {
			if auth == nil {
				c.Request.Header.Del(internalprobe.HeaderName)
			} else {
				auth.VerifyAndMarkRequest(c.Request)
			}
		}
		c.Next()
	}
}
