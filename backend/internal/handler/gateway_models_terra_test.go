//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type terraListingAccountRepo struct{ service.AccountRepository }

func (*terraListingAccountRepo) ListSchedulableByGroupID(context.Context, int64) ([]service.Account, error) {
	return []service.Account{{ID: 630, Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-6.1-terra": "gpt-6.1-terra"}},
		Extra:       map[string]any{"openai_passthrough": true},
	}}, nil
}

func TestModelsTerraVisibleForAllowlistedPassthroughGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewGatewayService(&terraListingAccountRepo{}, nil, nil, nil, nil, nil, nil, nil,
		&config.Config{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &GatewayHandler{gatewayService: svc}
	group := &service.Group{ID: 42, Platform: service.PlatformOpenAI,
		ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-6-astra", "gpt-6.1-terra"}}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &group.ID, Group: group})
	})
	r.GET("/v1/models", h.Models)
	r.GET("/models", h.Models)
	for _, path := range []string{"/v1/models", "/models"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code)
		var body struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		ids := make([]string, 0, len(body.Data))
		for _, model := range body.Data {
			ids = append(ids, model.ID)
		}
		require.ElementsMatch(t, []string{"gpt-6-astra", "gpt-6.1-terra"}, ids)
	}
}
