package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type docsRepoStub struct {
	value  string
	getErr error
	key    string
	saved  string
}

func (r *docsRepoStub) Get(context.Context, string) (*Setting, error) { panic("unexpected") }
func (r *docsRepoStub) GetValue(_ context.Context, key string) (string, error) {
	r.key = key
	return r.value, r.getErr
}
func (r *docsRepoStub) Set(_ context.Context, key, value string) error {
	r.key, r.saved = key, value
	return nil
}
func (r *docsRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected")
}
func (r *docsRepoStub) SetMultiple(context.Context, map[string]string) error { panic("unexpected") }
func (r *docsRepoStub) GetAll(context.Context) (map[string]string, error)    { panic("unexpected") }
func (r *docsRepoStub) Delete(context.Context, string) error                 { panic("unexpected") }

func TestSettingServiceGetDocsDefaultsAndPublishedFilter(t *testing.T) {
	repo := &docsRepoStub{getErr: ErrSettingNotFound}
	svc := NewSettingService(repo, &config.Config{})

	docs, err := svc.GetDocs(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, docs, 4)
	require.Equal(t, SettingKeyDocsContent, repo.key)
	require.Equal(t, "quick-start", docs[0].Slug)
}

func TestSettingServiceGetDocsFiltersDraftsAndKeepsOrder(t *testing.T) {
	repo := &docsRepoStub{value: `[{"id":"draft","slug":"draft","published":false,"title":{"zh":"草稿","en":"Draft"},"content":{"zh":"正文","en":"Body"}},{"id":"live","slug":"live","published":true,"title":{"zh":"发布","en":"Live"},"content":{"zh":"正文","en":"Body"}}]`}
	svc := NewSettingService(repo, &config.Config{})

	all, err := svc.GetDocs(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, []string{"draft", "live"}, []string{all[0].ID, all[1].ID})
	published, err := svc.GetDocs(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, published, 1)
	require.Equal(t, "live", published[0].ID)
}

func TestSettingServiceSaveDocsNormalizesAndUsesFixedKey(t *testing.T) {
	repo := &docsRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	docs := []DocArticle{{ID: " doc-1 ", Slug: " guide ", Published: true, Title: LocalizedDocText{Zh: " 标题 ", En: " Title "}, Content: LocalizedDocText{Zh: " 正文 ", En: " Body "}}}

	saved, err := svc.SaveDocs(context.Background(), docs)
	require.NoError(t, err)
	require.Equal(t, SettingKeyDocsContent, repo.key)
	require.Equal(t, "doc-1", saved[0].ID)
	require.Contains(t, repo.saved, `"slug":"guide"`)
}

func TestSettingServiceSaveDocsRejectsDuplicateSlugAndInvalidFields(t *testing.T) {
	svc := NewSettingService(&docsRepoStub{}, &config.Config{})
	valid := func(id, slug string) DocArticle {
		return DocArticle{ID: id, Slug: slug, Title: LocalizedDocText{Zh: "标题", En: "Title"}, Content: LocalizedDocText{Zh: "正文", En: "Body"}}
	}

	_, err := svc.SaveDocs(context.Background(), []DocArticle{valid("one", "same"), valid("two", "same")})
	require.Error(t, err)
	_, err = svc.SaveDocs(context.Background(), []DocArticle{valid("bad id", "Bad-Slug")})
	require.Error(t, err)
}
