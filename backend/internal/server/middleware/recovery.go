package middleware

import (
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// Recovery converts panics into the project's standard JSON error envelope.
//
// It preserves Gin's broken-pipe handling by not attempting to write a response
// when the client connection is already gone.
func Recovery() gin.HandlerFunc {
	// Gin's generic recovery consumes ErrAbortHandler and may finish a partial
	// download as a successful response. Handle that sentinel before logging or
	// writing JSON; net/http closes HTTP/1 connections or resets HTTP/2 streams.
	var logger *log.Logger
	if gin.DefaultErrorWriter != nil {
		logger = log.New(gin.DefaultErrorWriter, "[Recovery] ", log.LstdFlags)
	}
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			recoveredErr, _ := recovered.(error)
			if errors.Is(recoveredErr, http.ErrAbortHandler) {
				panic(http.ErrAbortHandler)
			}

			if isBrokenPipe(recoveredErr) {
				if recoveredErr != nil {
					_ = c.Error(recoveredErr)
				}
				c.Abort()
				return
			}
			if logger != nil {
				// Keep panic diagnostics without dumping request headers/credentials.
				logger.Printf("panic recovered: %v\n%s", recovered, debug.Stack())
			}

			if c.Writer.Written() {
				c.Abort()
				return
			}

			response.ErrorWithDetails(
				c,
				http.StatusInternalServerError,
				infraerrors.UnknownMessage,
				infraerrors.UnknownReason,
				nil,
			)
			c.Abort()
		}()
		c.Next()
	}
}

func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}

	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}

	var syscallErr *os.SyscallError
	if !errors.As(opErr.Err, &syscallErr) {
		return false
	}

	msg := strings.ToLower(syscallErr.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer")
}
