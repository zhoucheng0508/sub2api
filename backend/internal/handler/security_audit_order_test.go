package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type promptAuditOrderCase struct {
	file       string
	function   string
	auditToken string
}

func TestPromptAuditGatePrecedesAccountBillingAndUpstreamSideEffects(t *testing.T) {
	tests := []promptAuditOrderCase{
		{file: "gateway_handler.go", function: "Messages", auditToken: "checkSecurityAudit"},
		{file: "gateway_handler_chat_completions.go", function: "ChatCompletions", auditToken: "checkSecurityAudit"},
		{file: "gateway_handler_responses.go", function: "Responses", auditToken: "checkSecurityAudit"},
		{file: "gemini_v1beta_handler.go", function: "GeminiV1BetaModels", auditToken: "checkSecurityAudit"},
		{file: "openai_gateway_handler.go", function: "Responses", auditToken: "checkSecurityAudit"},
		{file: "openai_gateway_handler.go", function: "Messages", auditToken: "checkSecurityAudit"},
		{file: "openai_chat_completions.go", function: "ChatCompletions", auditToken: "checkSecurityAudit"},
		{file: "openai_images.go", function: "Images", auditToken: "checkSecurityAudit"},
		{file: "grok_media.go", function: "handleGrokMedia", auditToken: "checkSecurityAudit"},
		{file: "openai_embeddings.go", function: "Embeddings", auditToken: "checkSecurityAudit"},
		{file: "openai_alpha_search.go", function: "AlphaSearch", auditToken: "checkSecurityAudit"},
		{file: "image_task_handler.go", function: "Submit", auditToken: "checkSecurityAuditBeforeSubmit"},
		{file: "batch_image_handler.go", function: "Submit", auditToken: "checkSecurityAuditBeforeSubmit"},
		{file: "gateway_handler.go", function: "CountTokens", auditToken: "checkSecurityAudit"},
		{file: "openai_gateway_count_tokens.go", function: "CountTokens", auditToken: "checkSecurityAudit"},
	}
	sideEffectTokens := []string{
		"CheckBillingEligibility(", "SelectAccount", ".Forward", "acquireResponsesUserSlot(",
		"AcquireUserSlot", "TryAcquireUserSlot", "acquireImageGenerationSlot(",
		"h.tasks.Create(", "h.tasks.CreateWithResize(", "h.service.Submit(",
	}
	for _, tt := range tests {
		t.Run(tt.file+"/"+tt.function, func(t *testing.T) {
			functionSource := stripGoComments(goFunctionSource(t, tt.file, tt.function))
			auditIndex := strings.Index(functionSource, tt.auditToken)
			require.NotEqual(t, -1, auditIndex, "missing Prompt Audit gate")
			foundSideEffect := false
			for _, sideEffect := range sideEffectTokens {
				index := strings.Index(functionSource, sideEffect)
				if index < 0 {
					continue
				}
				foundSideEffect = true
				require.Lessf(t, auditIndex, index, "%s must run before %s", tt.auditToken, sideEffect)
			}
			require.True(t, foundSideEffect, "coverage case must contain a downstream side effect")
		})
	}
}

func TestAccountScopedAuditGateFollowsSelectionAndPrecedesUpstream(t *testing.T) {
	tests := []promptAuditOrderCase{
		{file: "gateway_handler.go", function: "Messages", auditToken: "checkSecurityAuditForAccount"},
		{file: "gateway_handler_chat_completions.go", function: "ChatCompletions", auditToken: "checkSecurityAuditForAccount"},
		{file: "gateway_handler_responses.go", function: "Responses", auditToken: "checkSecurityAuditForAccount"},
		{file: "gemini_v1beta_handler.go", function: "GeminiV1BetaModels", auditToken: "checkSecurityAuditForAccount"},
		{file: "openai_gateway_handler.go", function: "Responses", auditToken: "checkSecurityAuditForAccount"},
		{file: "openai_gateway_handler.go", function: "Messages", auditToken: "checkSecurityAuditForAccount"},
		{file: "openai_chat_completions.go", function: "ChatCompletions", auditToken: "checkSecurityAuditForAccount"},
		{file: "openai_images.go", function: "Images", auditToken: "checkSecurityAuditForAccount"},
		{file: "grok_media.go", function: "handleGrokMedia", auditToken: "checkSecurityAuditForAccount"},
		{file: "openai_embeddings.go", function: "Embeddings", auditToken: "checkSecurityAuditForAccount"},
		{file: "openai_alpha_search.go", function: "AlphaSearch", auditToken: "checkSecurityAuditForAccount"},
		{file: "openai_gateway_count_tokens.go", function: "CountTokens", auditToken: "checkSecurityAuditForAccount"},
	}
	for _, tt := range tests {
		t.Run(tt.file+"/"+tt.function, func(t *testing.T) {
			source := stripGoComments(goFunctionSource(t, tt.file, tt.function))
			selectionIndex := strings.Index(source, "account := selection.Account")
			auditIndex := strings.Index(source, tt.auditToken)
			forwardIndex := strings.Index(source, ".Forward")
			require.NotEqual(t, -1, selectionIndex, "missing selected account boundary")
			require.NotEqual(t, -1, auditIndex, "missing account-scoped audit gate")
			require.NotEqual(t, -1, forwardIndex, "missing upstream forward boundary")
			require.Less(t, selectionIndex, auditIndex)
			require.Less(t, auditIndex, forwardIndex)
		})
	}
}

func TestCountTokensAccountAuditFollowsSelectionAndPrecedesUpstream(t *testing.T) {
	tests := []struct {
		file           string
		selectionToken string
		forwardToken   string
	}{
		{file: "gateway_handler.go", selectionToken: "account, err := h.gatewayService.SelectAccountForModel", forwardToken: "ForwardCountTokens("},
		{file: "openai_gateway_count_tokens.go", selectionToken: "account, err := h.gatewayService.SelectAccountForTokenCount", forwardToken: "ForwardCountTokensAsAnthropic("},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			source := stripGoComments(goFunctionSource(t, tt.file, "CountTokens"))
			selectionIndex := strings.Index(source, tt.selectionToken)
			auditIndex := strings.Index(source, "checkSecurityAuditForAccount")
			forwardIndex := strings.Index(source, tt.forwardToken)
			require.NotEqual(t, -1, selectionIndex, "missing selected account boundary")
			require.NotEqual(t, -1, auditIndex, "missing account-scoped audit gate")
			require.NotEqual(t, -1, forwardIndex, "missing upstream forward boundary")
			require.Less(t, selectionIndex, auditIndex)
			require.Less(t, auditIndex, forwardIndex)
		})
	}
}

func TestOpenAICountTokensBlockedAuditDoesNotReleaseSelection(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "openai_gateway_count_tokens.go", "CountTokens"))
	auditIndex := strings.Index(source, "if decision := h.checkSecurityAuditForAccount")
	require.NotEqual(t, -1, auditIndex)
	afterAudit := source[auditIndex:]
	releaseIndex := strings.Index(afterAudit, "releaseSecurityAuditSelection(selection)")
	errorIndex := strings.Index(afterAudit, "h.anthropicSecurityAuditError(c, decision)")
	forwardIndex := strings.Index(afterAudit, "ForwardCountTokensAsAnthropic(")
	require.Equal(t, -1, releaseIndex, "count_tokens does not acquire an account slot")
	require.NotEqual(t, -1, errorIndex)
	require.NotEqual(t, -1, forwardIndex)
	require.Less(t, errorIndex, forwardIndex)
}

func TestAccountScopedAuditCallbacksPrecedeServiceManagedUpstream(t *testing.T) {
	tests := []promptAuditOrderCase{
		{file: "openai_live.go", function: "Live", auditToken: "BeforeAccountForward"},
		{file: "batch_image_handler.go", function: "checkSecurityAuditBeforeSubmit", auditToken: "BeforeAccountForward"},
	}
	for _, tt := range tests {
		t.Run(tt.file+"/"+tt.function, func(t *testing.T) {
			source := stripGoComments(goFunctionSource(t, tt.file, tt.function))
			auditIndex := strings.Index(source, "checkSecurityAuditForAccount")
			callbackIndex := strings.Index(source, tt.auditToken)
			require.NotEqual(t, -1, callbackIndex)
			require.NotEqual(t, -1, auditIndex)
			require.LessOrEqual(t, callbackIndex, auditIndex)
		})
	}
}

func TestWebSocketFirstTurnAccountAuditPrecedesCredentialLookup(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "openai_gateway_handler.go", "ResponsesWebSocket"))
	selectionIndex := strings.Index(source, "account := selection.Account")
	auditIndex := strings.Index(source, "checkSecurityAuditStageForAccount")
	credentialIndex := strings.Index(source, "GetRequestCredential")
	require.NotEqual(t, -1, selectionIndex)
	require.NotEqual(t, -1, auditIndex)
	require.NotEqual(t, -1, credentialIndex)
	require.Less(t, selectionIndex, auditIndex)
	require.Less(t, auditIndex, credentialIndex)
}

func stripGoComments(source string) string {
	source = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(source, "")
	return regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(source, "")
}

func goFunctionSource(t *testing.T, filename, functionName string) string {
	t.Helper()
	raw, err := os.ReadFile(filename)
	require.NoError(t, err)
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, filename, raw, 0)
	require.NoError(t, err)
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName || function.Body == nil {
			continue
		}
		start := files.Position(function.Pos()).Offset
		end := files.Position(function.End()).Offset
		require.Greater(t, end, start)
		return string(raw[start:end])
	}
	t.Fatalf("function %s not found in %s", functionName, filename)
	return ""
}
