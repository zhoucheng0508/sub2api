package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIPromptCacheKeyPrefix    = "sub2api-pcache-v1-"
	maxOpenAIAutoCacheBreakpoints = 4
	openAIPromptCacheExplicitJSON = `{"mode":"explicit"}`
)

type openAIPromptCacheDiagnostics struct {
	Mode                    openai_compat.PromptCacheMode
	KeySource               string
	KeyHash                 string
	BreakpointCount         int
	OptionsMode             string
	CacheFieldsAutoInjected bool
	AutoKeyInjected         bool
	AutoOptionsInjected     bool
	AutoBreakpointCount     int
	RetryStripped           bool
}

func isGPT56PromptCacheModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	return normalized == "gpt-5.6" || strings.HasPrefix(normalized, "gpt-5.6-")
}

func resolveOpenAIAPIKeyPromptCacheKey(
	c *gin.Context,
	account *Account,
	originalBody []byte,
	req *apicompat.ChatCompletionsRequest,
	upstreamModel string,
	legacyPromptCacheKey string,
) (string, openAIPromptCacheDiagnostics) {
	mode := openai_compat.ResolvePromptCacheMode(account.Extra)
	diagnostics := openAIPromptCacheDiagnostics{Mode: mode, KeySource: "none"}

	if req != nil {
		if clientKey := strings.TrimSpace(req.PromptCacheKey); clientKey != "" {
			diagnostics.KeySource = "client_body"
			diagnostics.KeyHash = hashSensitiveValueForLog(clientKey)
			return clientKey, diagnostics
		}
	}

	legacyPromptCacheKey = strings.TrimSpace(legacyPromptCacheKey)
	automationEnabled := mode == openai_compat.PromptCacheModeKeyOnly ||
		(mode == openai_compat.PromptCacheModeGPT56Explicit && isGPT56PromptCacheModel(upstreamModel))
	if !automationEnabled {
		if legacyPromptCacheKey != "" {
			diagnostics.KeySource = "session_header"
			diagnostics.KeyHash = hashSensitiveValueForLog(legacyPromptCacheKey)
		}
		return legacyPromptCacheKey, diagnostics
	}

	sessionID := explicitOpenAIHeaderSessionID(c)
	if sessionID == "" {
		for _, field := range []string{"conversation_id", "session_id"} {
			if value := strings.TrimSpace(gjson.GetBytes(originalBody, field).String()); value != "" {
				sessionID = value
				break
			}
		}
	}
	if sessionID == "" && legacyPromptCacheKey != "" {
		sessionID = legacyPromptCacheKey
	}

	key := deriveOpenAIAPIKeyPromptCacheKey(getAPIKeyIDFromContext(c), account.ID, upstreamModel, sessionID, req)
	if key == "" {
		return "", diagnostics
	}
	if sessionID != "" {
		diagnostics.KeySource = "session_header"
	} else {
		diagnostics.KeySource = "auto_derived"
	}
	diagnostics.KeyHash = hashSensitiveValueForLog(key)
	diagnostics.CacheFieldsAutoInjected = true
	diagnostics.AutoKeyInjected = true
	return key, diagnostics
}

func deriveOpenAIAPIKeyPromptCacheKey(apiKeyID, accountID int64, upstreamModel, sessionID string, req *apicompat.ChatCompletionsRequest) string {
	if accountID <= 0 || strings.TrimSpace(upstreamModel) == "" {
		return ""
	}
	if strings.TrimSpace(sessionID) == "" && !hasOpenAIStablePromptSeed(req) {
		return ""
	}
	h := sha256.New()
	writePromptCacheHashPart(h, "version", "1")
	writePromptCacheHashPart(h, "api_key_id", fmt.Sprintf("%d", apiKeyID))
	writePromptCacheHashPart(h, "account_id", fmt.Sprintf("%d", accountID))
	writePromptCacheHashPart(h, "model", strings.TrimSpace(upstreamModel))
	if sessionID != "" {
		writePromptCacheHashPart(h, "session", strings.TrimSpace(sessionID))
	} else {
		writeOpenAIStablePromptSeed(h, req)
	}
	sum := h.Sum(nil)
	// 40 hex characters keep the full key comfortably below common 64-byte
	// provider limits while retaining 160 bits of collision resistance.
	return openAIPromptCacheKeyPrefix + hex.EncodeToString(sum[:20])
}

func hasOpenAIStablePromptSeed(req *apicompat.ChatCompletionsRequest) bool {
	if req == nil {
		return false
	}
	if strings.TrimSpace(req.Instructions) != "" || len(req.Tools) > 0 || len(req.Functions) > 0 {
		return true
	}
	for _, message := range req.Messages {
		switch strings.ToLower(strings.TrimSpace(message.Role)) {
		case "system", "developer", "user":
			if len(message.Content) > 0 && string(message.Content) != `""` && string(message.Content) != "null" {
				return true
			}
		}
	}
	return false
}

type promptCacheHashWriter interface {
	Write([]byte) (int, error)
}

func writePromptCacheHashPart(h promptCacheHashWriter, name, value string) {
	_, _ = fmt.Fprintf(h, "%s:%d:", name, len(value))
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}

func writeOpenAIStablePromptSeed(h promptCacheHashWriter, req *apicompat.ChatCompletionsRequest) {
	if req == nil {
		writePromptCacheHashPart(h, "fallback", "empty")
		return
	}
	writePromptCacheHashPart(h, "instructions", req.Instructions)
	for _, message := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role == "system" || role == "developer" {
			writePromptCacheHashPart(h, "message_role", role)
			writePromptCacheHashPart(h, "message_content", string(message.Content))
		}
	}
	if tools, err := json.Marshal(req.Tools); err == nil {
		writePromptCacheHashPart(h, "tools", string(tools))
	}
	if functions, err := json.Marshal(req.Functions); err == nil {
		writePromptCacheHashPart(h, "functions", string(functions))
	}
	for _, message := range req.Messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "user") {
			writePromptCacheHashPart(h, "first_user", string(message.Content))
			break
		}
	}
}

func applyGPT56ExplicitPromptCache(req *apicompat.ResponsesRequest, diagnostics *openAIPromptCacheDiagnostics) error {
	if req == nil || diagnostics == nil || diagnostics.Mode != openai_compat.PromptCacheModeGPT56Explicit || !isGPT56PromptCacheModel(req.Model) {
		return nil
	}

	if len(req.PromptCacheOptions) == 0 {
		req.PromptCacheOptions = json.RawMessage(openAIPromptCacheExplicitJSON)
		diagnostics.AutoOptionsInjected = true
		diagnostics.CacheFieldsAutoInjected = true
	}
	diagnostics.OptionsMode = strings.TrimSpace(gjson.GetBytes(req.PromptCacheOptions, "mode").String())

	var input []map[string]any
	if err := json.Unmarshal(req.Input, &input); err != nil {
		return nil // string and non-message inputs are left untouched
	}
	existing := countOpenAIPromptCacheBreakpoints(input)
	remaining := maxOpenAIAutoCacheBreakpoints - existing
	if remaining > 0 {
		diagnostics.AutoBreakpointCount = addOpenAIPromptCacheBreakpoints(input, remaining)
		if diagnostics.AutoBreakpointCount > 0 {
			updated, err := json.Marshal(input)
			if err != nil {
				return fmt.Errorf("marshal prompt-cache input: %w", err)
			}
			req.Input = updated
			diagnostics.CacheFieldsAutoInjected = true
		}
	}
	diagnostics.BreakpointCount = countOpenAIPromptCacheBreakpoints(input)
	return nil
}

func countOpenAIPromptCacheBreakpoints(input []map[string]any) int {
	count := 0
	for _, item := range input {
		parts, _ := item["content"].([]any)
		for _, rawPart := range parts {
			part, _ := rawPart.(map[string]any)
			if _, ok := part["prompt_cache_breakpoint"]; ok {
				count++
			}
		}
	}
	return count
}

func addOpenAIPromptCacheBreakpoints(input []map[string]any, limit int) int {
	added := 0
	for itemIndex := len(input) - 1; itemIndex >= 0 && added < limit; itemIndex-- {
		item := input[itemIndex]
		role, _ := item["role"].(string)
		if role != "system" && role != "developer" && role != "user" {
			continue
		}
		switch content := item["content"].(type) {
		case string:
			if strings.TrimSpace(content) == "" {
				continue
			}
			item["content"] = []any{map[string]any{
				"type":                    "input_text",
				"text":                    content,
				"prompt_cache_breakpoint": map[string]any{"mode": "explicit"},
			}}
			added++
		case []any:
			for partIndex := len(content) - 1; partIndex >= 0 && added < limit; partIndex-- {
				part, _ := content[partIndex].(map[string]any)
				if part == nil || part["type"] != "input_text" || part["prompt_cache_breakpoint"] != nil {
					continue
				}
				text, _ := part["text"].(string)
				if strings.TrimSpace(text) == "" {
					continue
				}
				part["prompt_cache_breakpoint"] = map[string]any{"mode": "explicit"}
				added++
			}
		}
	}
	return added
}

func isAutoPromptCacheFieldRejection(statusCode int, responseBody []byte) bool {
	if statusCode != 400 || len(responseBody) == 0 {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(extractUpstreamErrorCode(responseBody)))
	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))
	param := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "error.param").String()))
	if strings.Contains(param, "prompt_cache_") {
		return true
	}
	if code != "unknown_parameter" && code != "unsupported_parameter" && !strings.Contains(message, "unknown parameter") && !strings.Contains(message, "unsupported parameter") {
		return false
	}
	return strings.Contains(param, "prompt_cache_") || strings.Contains(message, "prompt_cache_")
}
