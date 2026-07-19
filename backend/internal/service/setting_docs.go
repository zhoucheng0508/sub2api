package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeyDocsContent = "docs_content"
	MaxDocsRequestBytes   = 512 * 1024
	MaxDocsArticles       = 50
	maxDocIDLength        = 100
	maxDocSlugLength      = 100
	maxDocTitleLength     = 200
	maxDocContentLength   = 100000
)

var (
	docIDPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	docSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type LocalizedDocText struct {
	Zh string `json:"zh"`
	En string `json:"en"`
}

type DocArticle struct {
	ID        string           `json:"id"`
	Slug      string           `json:"slug"`
	Published bool             `json:"published"`
	Title     LocalizedDocText `json:"title"`
	Content   LocalizedDocText `json:"content"`
}

var defaultDocs = []DocArticle{
	{ID: "quick-start", Slug: "quick-start", Published: true, Title: LocalizedDocText{Zh: "快速开始", En: "Quick Start"}, Content: LocalizedDocText{Zh: "# 快速开始\n\nVote AI 提供统一的 AI API 网关。完成以下步骤即可发起第一次请求。\n\n## 接入步骤\n\n1. 注册并登录 Vote AI 控制台。\n2. 在密钥页面创建 API Key。\n3. 将客户端的 API 地址替换为控制台展示的 Endpoint。\n4. 选择可用模型并发送请求。\n\n> 请以控制台展示的实际 Endpoint、模型列表与计费记录为准。\n\n## 请求示例\n\n```bash\ncurl \"$API_ENDPOINT/v1/chat/completions\" \\\n  -H \"Authorization: Bearer $VOTE_AI_API_KEY\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"model\":\"your-model-id\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}]}'\n```\n\n不要在前端代码或公开仓库中暴露 API Key。", En: "# Quick Start\n\nVote AI provides one gateway for multiple AI providers. Follow these steps to make your first request.\n\n## Setup\n\n1. Create an account and sign in.\n2. Create an API key in the console.\n3. Replace your client's API endpoint with the Endpoint shown in the console.\n4. Select an available model and send a request.\n\n> Always use the current endpoint, model list, and billing records shown in the console.\n\n## Example request\n\n```bash\ncurl \"$API_ENDPOINT/v1/chat/completions\" \\\n  -H \"Authorization: Bearer $VOTE_AI_API_KEY\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"model\":\"your-model-id\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}]}'\n```\n\nNever expose an API key in frontend code or a public repository."}},
	{ID: "api-key", Slug: "api-key", Published: true, Title: LocalizedDocText{Zh: "获取 API Key", En: "Get an API Key"}, Content: LocalizedDocText{Zh: "# 获取 API Key\n\n登录控制台后进入 **API 密钥** 页面，创建一个仅用于当前应用的密钥。\n\n## 安全建议\n\n- 按应用或环境拆分密钥。\n- 仅通过环境变量读取密钥。\n- 怀疑泄露时立即删除并重新创建。\n- 定期查看用量记录，及时发现异常调用。\n\n```env\nVOTE_AI_API_KEY=your-api-key\nAPI_ENDPOINT=the-endpoint-shown-in-console\n```", En: "# Get an API Key\n\nSign in and open **API Keys** in the console. Create a separate key for each application.\n\n## Security recommendations\n\n- Separate keys by application or environment.\n- Load secrets from environment variables only.\n- Delete and rotate a key immediately if exposure is suspected.\n- Review usage records regularly for unexpected calls.\n\n```env\nVOTE_AI_API_KEY=your-api-key\nAPI_ENDPOINT=the-endpoint-shown-in-console\n```"}},
	{ID: "client-setup", Slug: "client-setup", Published: true, Title: LocalizedDocText{Zh: "客户端接入", En: "Client Setup"}, Content: LocalizedDocText{Zh: "# 客户端接入\n\n多数兼容客户端只需要配置 **API Key**、**Base URL** 和模型 ID。具体字段名称以客户端版本为准。\n\n## 通用配置\n\n| 配置项 | 内容 |\n| --- | --- |\n| API Key | 控制台创建的密钥 |\n| Base URL | 控制台展示的 API 地址 |\n| Model | 密钥可用模型中的模型 ID |\n\n## Node.js 示例\n\n```js\nconst response = await fetch(process.env.API_ENDPOINT + '/v1/chat/completions', {\n  method: 'POST',\n  headers: {\n    Authorization: 'Bearer ' + process.env.VOTE_AI_API_KEY,\n    'Content-Type': 'application/json'\n  },\n  body: JSON.stringify({ model: 'your-model-id', messages: [{ role: 'user', content: 'Hello' }] })\n})\n```", En: "# Client Setup\n\nMost compatible clients only require an **API key**, **Base URL**, and model ID. Field names may vary by client version.\n\n## Common settings\n\n| Setting | Value |\n| --- | --- |\n| API Key | A key created in the console |\n| Base URL | The API endpoint shown in the console |\n| Model | A model ID available to the key |\n\n## Node.js example\n\n```js\nconst response = await fetch(process.env.API_ENDPOINT + '/v1/chat/completions', {\n  method: 'POST',\n  headers: { Authorization: 'Bearer ' + process.env.VOTE_AI_API_KEY, 'Content-Type': 'application/json' },\n  body: JSON.stringify({ model: 'your-model-id', messages: [{ role: 'user', content: 'Hello' }] })\n})\n```"}},
	{ID: "faq", Slug: "faq", Published: true, Title: LocalizedDocText{Zh: "常见问题", En: "FAQ"}, Content: LocalizedDocText{Zh: "# 常见问题\n\n## 返回 401 怎么办？\n\n确认 API Key 完整、未被删除，并使用 `Bearer <API_KEY>` 格式发送 Authorization 请求头。\n\n## 找不到模型怎么办？\n\n请从控制台的可用模型列表复制模型 ID。不同密钥可能拥有不同的可用范围。\n\n## 请求超时怎么办？\n\n先检查本地网络与 Endpoint 配置，再适当增加客户端超时时间。持续异常时请携带请求时间和错误信息联系支持。\n\n## 如何查看费用？\n\n控制台的用量记录会展示调用与扣费信息，最终结果以该记录为准。", En: "# Frequently Asked Questions\n\n## Why do I receive a 401?\n\nVerify that the API key is complete and active, and send it as `Authorization: Bearer <API_KEY>`.\n\n## Why is a model unavailable?\n\nCopy the model ID from the available model list in the console. Availability can differ between keys.\n\n## What should I do about timeouts?\n\nCheck your network and endpoint first, then increase the client timeout if needed. Contact support with the request time and error details if the issue continues.\n\n## Where can I review charges?\n\nThe console usage records show requests and charges. Those records are the final source of truth."}},
}

func (s *SettingService) GetDocs(ctx context.Context, publishedOnly bool) ([]DocArticle, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDocsContent)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, fmt.Errorf("get docs setting: %w", err)
	}

	docs := cloneDocs(defaultDocs)
	if strings.TrimSpace(value) != "" {
		var stored []DocArticle
		if json.Unmarshal([]byte(value), &stored) == nil && validateAndNormalizeDocs(stored) == nil {
			docs = stored
		}
	}
	if !publishedOnly {
		return docs, nil
	}

	published := make([]DocArticle, 0, len(docs))
	for _, doc := range docs {
		if doc.Published {
			published = append(published, doc)
		}
	}
	return published, nil
}

func (s *SettingService) SaveDocs(ctx context.Context, docs []DocArticle) ([]DocArticle, error) {
	if err := validateAndNormalizeDocs(docs); err != nil {
		return nil, infraerrors.BadRequest("INVALID_DOCS", err.Error())
	}
	encoded, err := json.Marshal(docs)
	if err != nil {
		return nil, fmt.Errorf("encode docs: %w", err)
	}
	if len(encoded) > MaxDocsRequestBytes {
		return nil, infraerrors.BadRequest("DOCS_TOO_LARGE", fmt.Sprintf("documents exceed %d bytes", MaxDocsRequestBytes))
	}
	if err := s.settingRepo.Set(ctx, SettingKeyDocsContent, string(encoded)); err != nil {
		return nil, fmt.Errorf("save docs setting: %w", err)
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return docs, nil
}

func validateAndNormalizeDocs(docs []DocArticle) error {
	if len(docs) > MaxDocsArticles {
		return fmt.Errorf("too many documents (max %d)", MaxDocsArticles)
	}
	ids := make(map[string]struct{}, len(docs))
	slugs := make(map[string]struct{}, len(docs))
	for i := range docs {
		doc := &docs[i]
		doc.ID = strings.TrimSpace(doc.ID)
		doc.Slug = strings.TrimSpace(doc.Slug)
		doc.Title.Zh = strings.TrimSpace(doc.Title.Zh)
		doc.Title.En = strings.TrimSpace(doc.Title.En)
		doc.Content.Zh = strings.TrimSpace(doc.Content.Zh)
		doc.Content.En = strings.TrimSpace(doc.Content.En)

		if doc.ID == "" || len(doc.ID) > maxDocIDLength || !docIDPattern.MatchString(doc.ID) {
			return fmt.Errorf("document %d has invalid id", i+1)
		}
		if _, exists := ids[doc.ID]; exists {
			return fmt.Errorf("document %d has duplicate id", i+1)
		}
		ids[doc.ID] = struct{}{}
		if doc.Slug == "" || len(doc.Slug) > maxDocSlugLength || !docSlugPattern.MatchString(doc.Slug) {
			return fmt.Errorf("document %d has invalid slug", i+1)
		}
		if _, exists := slugs[doc.Slug]; exists {
			return fmt.Errorf("document %d has duplicate slug", i+1)
		}
		slugs[doc.Slug] = struct{}{}
		if doc.Title.Zh == "" || doc.Title.En == "" || len(doc.Title.Zh) > maxDocTitleLength || len(doc.Title.En) > maxDocTitleLength {
			return fmt.Errorf("document %d has invalid title", i+1)
		}
		if doc.Content.Zh == "" || doc.Content.En == "" || len(doc.Content.Zh) > maxDocContentLength || len(doc.Content.En) > maxDocContentLength {
			return fmt.Errorf("document %d has invalid content", i+1)
		}
	}
	return nil
}

func cloneDocs(docs []DocArticle) []DocArticle {
	cloned := make([]DocArticle, len(docs))
	copy(cloned, docs)
	return cloned
}
