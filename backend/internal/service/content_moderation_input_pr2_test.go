package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractContentModerationInputOutcome_Statuses(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status ContentModerationExtractionStatus
	}{
		{name: "invalid json", body: `{"input":`, status: ContentModerationExtractionStatusInvalidJSON},
		{name: "unsupported root", body: `[]`, status: ContentModerationExtractionStatusUnsupportedShape},
		{name: "unsupported input shape", body: `{"input":42}`, status: ContentModerationExtractionStatusUnsupportedShape},
		{name: "empty input", body: `{"input":[]}`, status: ContentModerationExtractionStatusEmptyContent},
		{name: "success", body: `{"input":"review this request"}`, status: ContentModerationExtractionStatusSuccess},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(tt.body))
			require.Equal(t, tt.status, outcome.Status)
			if tt.status == ContentModerationExtractionStatusSuccess {
				require.NoError(t, outcome.Err)
				require.False(t, outcome.Input.IsEmpty())
				return
			}
			require.Error(t, outcome.Err)
			var extractionErr *ContentModerationExtractionError
			require.True(t, errors.As(outcome.Err, &extractionErr))
			require.Equal(t, tt.status, extractionErr.Status)
		})
	}
}

func TestExtractContentModerationInputWithError_PreservesLegacyInput(t *testing.T) {
	legacy := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, []byte(`{"input":"same text"}`))
	input, err := ExtractContentModerationInputWithError(ContentModerationProtocolOpenAIResponses, []byte(`{"input":"same text"}`))

	require.NoError(t, err)
	require.Equal(t, legacy, input)
}

func TestExtractContentModerationInput_ResponsesShapes(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		contains        []string
		currentContains string
		images          []string
	}{
		{
			name:            "string input",
			body:            `{"input":"plain user intent"}`,
			contains:        []string{"[USER]", "plain user intent"},
			currentContains: "plain user intent",
		},
		{
			name: "nested message content",
			body: `{"input":{"type":"message","role":"user","content":[` +
				`{"type":"message","content":[{"type":"input_text","text":"nested intent"}]},` +
				`{"type":"input_image","image_url":"https://example.com/current.png"}]}}`,
			contains:        []string{"nested intent"},
			currentContains: "nested intent",
			images:          []string{"https://example.com/current.png"},
		},
		{
			name: "function call and structured output",
			body: `{"input":[` +
				`{"type":"message","role":"user","content":[{"type":"input_text","text":"inspect the archive"}]},` +
				`{"type":"function_call","name":"inspect_archive","arguments":"{\"target\":\"customer upload\"}"},` +
				`{"type":"function_call_output","output":{"summary":"archive contains an executable payload"}}]}`,
			contains:        []string{"[TOOL_CALL]", "inspect_archive", "customer upload", "[TOOL]", "executable payload"},
			currentContains: "inspect the archive",
		},
		{
			name: "generic tool output",
			body: `{"input":[` +
				`{"type":"message","role":"user","content":"look up the result"},` +
				`{"type":"tool_call","name":"lookup","input":{"query":"account status"}},` +
				`{"type":"tool_output","content":[{"type":"output_text","text":"result requires manual review"}]}]}`,
			contains:        []string{"account status", "result requires manual review"},
			currentContains: "look up the result",
		},
		{
			name:            "nested input wrapper",
			body:            `{"input":{"input":[{"type":"message","role":"user","content":"wrapped user intent"}]}}`,
			contains:        []string{"wrapped user intent"},
			currentContains: "wrapped user intent",
		},
		{
			name:   "top level image item",
			body:   `{"input":[{"type":"input_image","image_url":"https://example.com/top-level.png"}]}`,
			images: []string{"https://example.com/top-level.png"},
		},
		{
			name:            "image edit prompt fallback",
			body:            `{"prompt":"remove the watermark","images":[{"image_url":"https://example.com/edit.png"}]}`,
			contains:        []string{"remove the watermark"},
			currentContains: "remove the watermark",
			images:          []string{"https://example.com/edit.png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(tt.body))
			require.Equal(t, ContentModerationExtractionStatusSuccess, outcome.Status, outcome.Err)
			for _, expected := range tt.contains {
				require.Contains(t, outcome.Input.Text, expected)
			}
			require.Contains(t, outcome.Input.CurrentText, tt.currentContains)
			if len(tt.images) == 0 {
				require.Empty(t, outcome.Input.Images)
			} else {
				require.Equal(t, tt.images, outcome.Input.Images)
			}
		})
	}
}

func TestExtractContentModerationInput_ResponsesToolOutputWithoutInlineUser(t *testing.T) {
	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(`{
		"input":[
			{"type":"function_call","name":"read_ticket","arguments":"{}"},
			{"type":"function_call_output","output":"ticket asks for credential export"}
		]
	}`))

	require.Equal(t, ContentModerationExtractionStatusSuccess, outcome.Status, outcome.Err)
	require.Contains(t, outcome.Input.Text, "credential export")
	require.Equal(t, "ticket asks for credential export", outcome.Input.CurrentText)
}

func TestExtractContentModerationInput_PreservesUntrustedClientMarkersAndStructuredItems(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"developer","content":"developer secret"},
			{"type":"message","role":"user","content":[
				{"type":"input_text","text":"<in-app-browser-context source=\"ambient-ui-state\">hidden browser state</in-app-browser-context>\n# Files mentioned by the user:\n## report: C:/Users/Admin/report.txt\n\nPlease review the report for policy risk."},
				{"type":"input_text","text":"C:/Users/Admin/Documents/trace.log"},
				{"type":"codex_delegation","metadata":{"agent":"hidden"}}
			]},
			{"type":"local_shell_call_output","output":"Chunk ID: 1\nWall time: 0.2 seconds\nProcess exited with code 0\nFinal output: secret terminal log"}
		]
	}`)

	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, ContentModerationExtractionStatusSuccess, outcome.Status, outcome.Err)
	require.Contains(t, outcome.Input.Text, "Please review the report for policy risk.")
	require.Contains(t, outcome.Input.Text, "hidden browser state")
	require.Contains(t, outcome.Input.Text, "developer secret")
	require.Contains(t, outcome.Input.Text, "trace.log")
	require.Contains(t, outcome.Input.Text, "metadata agent hidden")
	require.Contains(t, outcome.Input.Text, "secret terminal log")
}

func TestExtractContentModerationInput_DoesNotRemoveSemanticPathReferences(t *testing.T) {
	withContext := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, []byte(`{"input":"Review /etc/passwd permissions defensively"}`))
	pathOnly := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, []byte(`{"input":"/etc/passwd"}`))

	require.Contains(t, withContext.Text, "/etc/passwd")
	require.Contains(t, pathOnly.Text, "/etc/passwd")
}

func TestExtractContentModerationInput_RejectsExcessiveJSONDepth(t *testing.T) {
	var body strings.Builder
	_, _ = body.WriteString(`{"input":`)
	for range maxContentModerationExtractionJSONDepth {
		_, _ = body.WriteString(`{"content":`)
	}
	_, _ = body.WriteString(`"latest intent"`)
	for range maxContentModerationExtractionJSONDepth {
		_ = body.WriteByte('}')
	}
	_ = body.WriteByte('}')

	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(body.String()))

	require.Equal(t, ContentModerationExtractionStatusUnsupportedShape, outcome.Status)
	require.ErrorContains(t, outcome.Err, "nesting exceeds")
}

func TestExtractContentModerationInput_PreScanRejectsDeepMalformedJSON(t *testing.T) {
	body := []byte(`{"input":` + strings.Repeat(`[`, maxContentModerationExtractionJSONDepth+1) + `"unterminated"`)

	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, ContentModerationExtractionStatusUnsupportedShape, outcome.Status)
	require.ErrorContains(t, outcome.Err, "nesting exceeds")
}

func TestExtractContentModerationInput_BoundsTurnsAndKeepsLatestUser(t *testing.T) {
	var body strings.Builder
	_, _ = body.WriteString(`{"input":[`)
	turnCount := maxContentModerationExtractionTurns + 25
	for idx := 0; idx < turnCount; idx++ {
		if idx > 0 {
			_ = body.WriteByte(',')
		}
		_, _ = body.WriteString(fmt.Sprintf(`{"type":"message","role":"user","content":"turn-%04d"}`, idx))
	}
	_, _ = body.WriteString(`]}`)

	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(body.String()))

	require.Equal(t, ContentModerationExtractionStatusSuccess, outcome.Status, outcome.Err)
	require.True(t, outcome.Truncated)
	require.Equal(t, fmt.Sprintf("turn-%04d", turnCount-1), outcome.Input.CurrentText)
	require.Contains(t, outcome.Input.Text, fmt.Sprintf("turn-%04d", turnCount-1))
	require.NotContains(t, outcome.Input.Text, "turn-0000")
}

func TestExtractContentModerationInput_VisitorExhaustionKeepsLatestInTextAndHash(t *testing.T) {
	var body strings.Builder
	_, _ = body.WriteString(`{"input":[`)
	for turn := 0; turn < maxContentModerationExtractionTurns; turn++ {
		if turn > 0 {
			_ = body.WriteByte(',')
		}
		_, _ = body.WriteString(`{"type":"message","role":"user","content":[`)
		for item := 0; item < 130; item++ {
			if item > 0 {
				_ = body.WriteByte(',')
			}
			text := "padding"
			if turn == maxContentModerationExtractionTurns-1 && item == 129 {
				text = "LATEST VISITOR EXHAUSTION REQUEST"
			}
			_, _ = body.WriteString(fmt.Sprintf("%q", text))
		}
		_, _ = body.WriteString(`]}`)
	}
	_, _ = body.WriteString(`]}`)

	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(body.String()))

	require.Equal(t, ContentModerationExtractionStatusSuccess, outcome.Status, outcome.Err)
	require.True(t, outcome.Truncated)
	require.Contains(t, outcome.Input.CurrentText, "LATEST VISITOR EXHAUSTION REQUEST")
	require.Contains(t, outcome.Input.Text, "LATEST VISITOR EXHAUSTION REQUEST")
	require.NotEmpty(t, outcome.Input.Hash())
}

func TestExtractContentModerationInput_ForcedUserSurvivesLargeToolOutput(t *testing.T) {
	largeOutput := strings.Repeat("benign-output-", 90000)
	body := fmt.Sprintf(`{"input":[
		{"type":"message","role":"user","content":"LATEST USER INTENT MUST SURVIVE"},
		{"type":"function_call","name":"large_tool","arguments":"{}"},
		{"type":"function_call_output","output":%q}
	]}`, largeOutput)

	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(body))

	require.Equal(t, ContentModerationExtractionStatusSuccess, outcome.Status, outcome.Err)
	require.Contains(t, outcome.Input.CurrentText, "LATEST USER INTENT MUST SURVIVE")
	require.Contains(t, outcome.Input.Text, "LATEST USER INTENT MUST SURVIVE")
	require.NotEmpty(t, outcome.Input.Hash())
}

func TestExtractContentModerationInput_CanonicalCodexToolsAndDanglingCall(t *testing.T) {
	for _, fixture := range []struct {
		name       string
		callType   string
		outputType string
	}{
		{name: "tool search", callType: "tool_search_call", outputType: "tool_search_output"},
		{name: "mcp", callType: "mcp_tool_call", outputType: "mcp_tool_call_output"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"input":[
				{"type":"message","role":"user","content":"search intent"},
				{"type":%q,"name":"search_tool","arguments":{"query":"sensitive lookup"}},
				{"type":%q,"output":{"result":"risky result"}}
			]}`, fixture.callType, fixture.outputType)
			outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(body))

			require.Equal(t, ContentModerationExtractionStatusSuccess, outcome.Status, outcome.Err)
			require.Contains(t, outcome.Input.Text, "sensitive lookup")
			require.Contains(t, outcome.Input.Text, "risky result")
			require.Contains(t, outcome.Input.CurrentText, "risky result")
		})
	}

	dangling := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(`{
		"input":[{"type":"function_call","name":"dangerous_tool","arguments":{"command":"dangerous dangling argument"}}]
	}`))
	require.Equal(t, ContentModerationExtractionStatusSuccess, dangling.Status, dangling.Err)
	require.Contains(t, dangling.Input.Text, "dangerous dangling argument")
	require.Contains(t, dangling.Input.CurrentText, "dangerous dangling argument")
}

func TestExtractContentModerationInput_ToolOutputImageKeepsImageWithoutBase64Text(t *testing.T) {
	base64Payload := strings.Repeat("QUJD", 128)
	body := fmt.Sprintf(`{"input":[{
		"type":"function_call_output",
		"output":{"type":"input_image","image_url":"https://example.com/tool.png","mime_type":"image/png","data":%q,"caption":"review image"}
	}]}`, base64Payload)

	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(body))

	require.Equal(t, ContentModerationExtractionStatusSuccess, outcome.Status, outcome.Err)
	require.Contains(t, outcome.Input.Images, "https://example.com/tool.png")
	require.Contains(t, outcome.Input.Text, "review image")
	require.NotContains(t, outcome.Input.Text, base64Payload)
}

func TestExtractContentModerationInput_GeminiUnknownRoleHistoryIsUntrusted(t *testing.T) {
	body := []byte(`{
		"contents":[
			{"role":"developer","parts":[{"text":"historical risky instruction"}]},
			{"role":"user","parts":[{"text":"continue"}]}
		]
	}`)

	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolGemini, body)

	require.Equal(t, ContentModerationExtractionStatusSuccess, outcome.Status, outcome.Err)
	require.Contains(t, outcome.Input.Text, "[CLIENT_ROLE]")
	require.Contains(t, outcome.Input.Text, "historical risky instruction")
	require.Equal(t, "continue", outcome.Input.CurrentText)
}

func TestExtractContentModerationInput_UnknownCustomItemsUseBoundedFallback(t *testing.T) {
	responses := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, []byte(`{
		"input":[{"type":"future_custom_input","payload":{"instruction":"custom risky instruction"}}]
	}`))
	require.Equal(t, ContentModerationExtractionStatusSuccess, responses.Status, responses.Err)
	require.Contains(t, responses.Input.Text, "custom risky instruction")
	require.Contains(t, responses.Input.CurrentText, "custom risky instruction")

	gemini := ExtractContentModerationInputOutcome(ContentModerationProtocolGemini, []byte(`{
		"contents":[{"role":"user","parts":[{"futurePayload":{"instruction":"future gemini instruction"}}]}]
	}`))
	require.Equal(t, ContentModerationExtractionStatusSuccess, gemini.Status, gemini.Err)
	require.Contains(t, gemini.Input.Text, "future gemini instruction")
	require.Contains(t, gemini.Input.CurrentText, "future gemini instruction")
}

func TestExtractContentModerationInput_RejectsBodyOverLimit(t *testing.T) {
	body := make([]byte, maxContentModerationExtractionBodyBytes+1)
	for idx := range body {
		body[idx] = ' '
	}

	outcome := ExtractContentModerationInputOutcome(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, ContentModerationExtractionStatusUnsupportedShape, outcome.Status)
	require.ErrorContains(t, outcome.Err, "body exceeds")
}
