package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type docsHandlerRepoStub struct {
	value string
	key   string
}

func (r *docsHandlerRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected")
}
func (r *docsHandlerRepoStub) GetValue(context.Context, string) (string, error) { return r.value, nil }
func (r *docsHandlerRepoStub) Set(_ context.Context, key, value string) error {
	r.key, r.value = key, value
	return nil
}
func (r *docsHandlerRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected")
}
func (r *docsHandlerRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected")
}
func (r *docsHandlerRepoStub) GetAll(context.Context) (map[string]string, error) { panic("unexpected") }
func (r *docsHandlerRepoStub) Delete(context.Context, string) error              { panic("unexpected") }

func TestSettingHandlerUpdateDocs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &docsHandlerRepoStub{}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), nil, nil, nil, nil, nil, nil)
	body := `[{"id":" guide ","slug":" quick-start ","published":true,"title":{"zh":" 标题 ","en":" Title "},"content":{"zh":" 正文 ","en":" Body "}}]`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/docs", strings.NewReader(body))

	h.UpdateDocs(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, service.SettingKeyDocsContent, repo.key)
	var responseBody struct {
		Data []service.DocArticle `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
	require.Equal(t, "guide", responseBody.Data[0].ID)
}

func TestSettingHandlerUpdateDocsRejectsUnknownFieldAndOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSettingHandler(service.NewSettingService(&docsHandlerRepoStub{}, &config.Config{}), nil, nil, nil, nil, nil, nil)

	for _, body := range []string{
		`[{"id":"guide","slug":"guide","published":true,"title":{"zh":"标题","en":"Title"},"content":{"zh":"正文","en":"Body"},"key":"other"}]`,
		strings.Repeat("x", service.MaxDocsRequestBytes+1),
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/docs", strings.NewReader(body))
		h.UpdateDocs(c)
		require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest)
	}
}
