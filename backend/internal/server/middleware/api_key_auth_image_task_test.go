package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsGeneratedTaskRead(t *testing.T) {
	for _, path := range []string{
		"/v1/images/tasks/imgtask_123",
		"/images/tasks/imgtask_123",
		"/v1/media/videos/media_123",
		"/v1/media/videos/media_123/content",
		"/media/videos/media_123",
		"/media/videos/media_123/content",
	} {
		require.True(t, isGeneratedTaskRead(http.MethodGet, path), path)
	}

	require.False(t, isGeneratedTaskRead(http.MethodPost, "/v1/media/videos/media_123"))
	require.False(t, isGeneratedTaskRead(http.MethodGet, "/v1/media/videos"))
	require.False(t, isGeneratedTaskRead(http.MethodGet, "/v1/media/models"))
	require.False(t, isGeneratedTaskRead(http.MethodGet, "/v1/images/generations"))
}
