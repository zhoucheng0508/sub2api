package service

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/tidwall/gjson"
)

const (
	maxContentModerationExtractionBodyBytes        = 16 << 20
	maxContentModerationExtractionJSONDepth        = 32
	maxContentModerationExtractionJSONFields       = 32768
	maxContentModerationExtractionStructuralTokens = 131072
	maxContentModerationExtractionTurns            = 512
	maxContentModerationExtractionContentItems     = 2048
	maxContentModerationExtractionVisitedValues    = 65536
)

type ContentModerationExtractionStatus string

const (
	ContentModerationExtractionStatusSuccess          ContentModerationExtractionStatus = "success"
	ContentModerationExtractionStatusInvalidJSON      ContentModerationExtractionStatus = "invalid_json"
	ContentModerationExtractionStatusUnsupportedShape ContentModerationExtractionStatus = "unsupported_shape"
	ContentModerationExtractionStatusEmptyContent     ContentModerationExtractionStatus = "empty_content"
)

// ContentModerationExtractionError lets callers distinguish malformed JSON,
// unsupported request envelopes, and valid requests without auditable content.
type ContentModerationExtractionError struct {
	Status ContentModerationExtractionStatus
	Detail string
}

func (e *ContentModerationExtractionError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Detail) == "" {
		return string(e.Status)
	}
	return fmt.Sprintf("%s: %s", e.Status, e.Detail)
}

type ContentModerationExtractionOutcome struct {
	Input     ContentModerationInput
	Status    ContentModerationExtractionStatus
	Truncated bool
	Err       error
}

func ExtractContentModerationText(protocol string, body []byte) string {
	return ExtractContentModerationInput(protocol, body).Text
}

// ExtractContentModerationInput preserves the legacy API. New callers that
// need diagnostic detail should use ExtractContentModerationInputOutcome.
func ExtractContentModerationInput(protocol string, body []byte) ContentModerationInput {
	return ExtractContentModerationInputOutcome(protocol, body).Input
}

func ExtractContentModerationInputWithError(protocol string, body []byte) (ContentModerationInput, error) {
	outcome := ExtractContentModerationInputOutcome(protocol, body)
	if outcome.Err != nil {
		return outcome.Input, outcome.Err
	}
	return outcome.Input, nil
}

func ExtractContentModerationInputOutcome(protocol string, body []byte) ContentModerationExtractionOutcome {
	if len(body) > maxContentModerationExtractionBodyBytes {
		return contentModerationExtractionFailure(ContentModerationExtractionStatusUnsupportedShape, "request body exceeds extraction limit")
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return contentModerationExtractionFailure(ContentModerationExtractionStatusEmptyContent, "request body is empty")
	}
	if detail := validateContentModerationJSONStructure(body); detail != "" {
		return contentModerationExtractionFailure(ContentModerationExtractionStatusUnsupportedShape, detail)
	}
	if !gjson.ValidBytes(body) {
		return contentModerationExtractionFailure(ContentModerationExtractionStatusInvalidJSON, "request body is not valid JSON")
	}

	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return contentModerationExtractionFailure(ContentModerationExtractionStatusUnsupportedShape, "request body must be a JSON object")
	}

	budget := &contentModerationExtractionBudget{}
	var parts []string
	var images []string
	var currentText string
	shapeSupported := false

	switch protocol {
	case ContentModerationProtocolAnthropicMessages:
		messages := root.Get("messages")
		shapeSupported = messages.IsArray()
		if shapeSupported {
			currentText = collectAnthropicConversation(messages, &parts, &images, budget)
		}
	case ContentModerationProtocolOpenAIChat:
		messages := root.Get("messages")
		shapeSupported = messages.IsArray()
		if shapeSupported {
			currentText = collectRoleConversation(messages, &parts, &images, budget)
		}
	case ContentModerationProtocolOpenAIResponses:
		input := root.Get("input")
		shapeSupported = isSupportedResponsesInput(input)
		if shapeSupported {
			currentText = collectResponsesConversation(input, &parts, &images, budget)
		} else if prompt := root.Get("prompt"); prompt.Type == gjson.String {
			// Some image-edit clients route JSON envelopes through /v1/responses.
			shapeSupported = true
			var promptParts []string
			addModerationText(&promptParts, prompt.String())
			currentText = appendModerationTurn(&parts, "user", promptParts)
			collectImageFields(root, &images, budget)
		}
	case ContentModerationProtocolGemini:
		contents := root.Get("contents")
		shapeSupported = contents.IsArray()
		if shapeSupported {
			currentText = collectGeminiConversation(contents, &parts, &images, budget)
		}
	case ContentModerationProtocolOpenAIImages:
		prompt := root.Get("prompt")
		shapeSupported = prompt.Type == gjson.String || hasContentModerationImageField(root)
		if prompt.Type == gjson.String {
			addModerationText(&parts, prompt.String())
			currentText = moderationPartsText(parts)
		}
		collectImageFields(root, &images, budget)
	default:
		input := root.Get("input")
		messages := root.Get("messages")
		contents := root.Get("contents")
		prompt := root.Get("prompt")
		switch {
		case isSupportedResponsesInput(input):
			shapeSupported = true
			currentText = collectResponsesConversation(input, &parts, &images, budget)
		case messages.IsArray():
			shapeSupported = true
			currentText = collectRoleConversation(messages, &parts, &images, budget)
		case contents.IsArray():
			shapeSupported = true
			currentText = collectGeminiConversation(contents, &parts, &images, budget)
		case prompt.Type == gjson.String:
			shapeSupported = true
			addModerationText(&parts, prompt.String())
			currentText = moderationPartsText(parts)
			collectImageFields(root, &images, budget)
		}
	}

	if !shapeSupported {
		return contentModerationExtractionFailure(ContentModerationExtractionStatusUnsupportedShape, "protocol request envelope is not supported")
	}

	extractedText := strings.TrimSpace(strings.Join(parts, "\n\n"))
	currentText = normalizeContentModerationText(currentText)
	if currentText != "" && (!strings.Contains(extractedText, currentText) || len([]rune(extractedText)) > maxModerationInputRunes) {
		extractedText = strings.TrimSpace("[CURRENT]\n" + currentText + "\n\n" + extractedText)
	}
	out := ContentModerationInput{
		Text:        extractedText,
		CurrentText: currentText,
		Images:      normalizeModerationImages(images),
	}
	out.Normalize()
	if out.IsEmpty() {
		outcome := contentModerationExtractionFailure(ContentModerationExtractionStatusEmptyContent, "request contains no auditable user or tool content")
		outcome.Truncated = budget.truncated
		return outcome
	}
	return ContentModerationExtractionOutcome{
		Input:     out,
		Status:    ContentModerationExtractionStatusSuccess,
		Truncated: budget.truncated,
	}
}

func contentModerationExtractionFailure(status ContentModerationExtractionStatus, detail string) ContentModerationExtractionOutcome {
	err := &ContentModerationExtractionError{Status: status, Detail: detail}
	return ContentModerationExtractionOutcome{Status: status, Err: err}
}

func validateContentModerationJSONStructure(body []byte) string {
	depth := 0
	fields := 0
	structuralTokens := 0
	inString := false
	escaped := false
	for _, ch := range body {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			depth++
			structuralTokens++
			if depth > maxContentModerationExtractionJSONDepth {
				return "JSON nesting exceeds extraction limit"
			}
		case '}', ']':
			depth--
			structuralTokens++
		case ':':
			fields++
			structuralTokens++
			if fields > maxContentModerationExtractionJSONFields {
				return "JSON field count exceeds extraction limit"
			}
		case ',':
			structuralTokens++
		}
		if structuralTokens > maxContentModerationExtractionStructuralTokens {
			return "JSON structure exceeds extraction limit"
		}
	}
	return ""
}

type contentModerationExtractionBudget struct {
	visited   int
	truncated bool
}

func (b *contentModerationExtractionBudget) visit(depth int) bool {
	if b == nil {
		return true
	}
	if depth > maxContentModerationExtractionJSONDepth || b.visited >= maxContentModerationExtractionVisitedValues {
		b.truncated = true
		return false
	}
	b.visited++
	return true
}

func (b *contentModerationExtractionBudget) exhausted() bool {
	return b != nil && b.visited >= maxContentModerationExtractionVisitedValues
}

func mergeContentModerationExtractionBudget(target, source *contentModerationExtractionBudget) {
	if target != nil && source != nil && source.truncated {
		target.truncated = true
	}
}

func appendModerationTurn(parts *[]string, role string, turnParts []string) string {
	text := moderationPartsText(turnParts)
	if text == "" {
		return ""
	}
	*parts = append(*parts, fmt.Sprintf("[%s]\n%s", strings.ToUpper(role), text))
	return text
}

func appendModerationLabeledTurn(parts *[]string, label string, turnParts []string) string {
	text := moderationPartsText(turnParts)
	if text == "" {
		return ""
	}
	*parts = append(*parts, fmt.Sprintf("[%s]\n%s", strings.ToUpper(label), text))
	return text
}

func appendNewestModerationTurns(parts *[]string, newestFirst []string) {
	for idx := len(newestFirst) - 1; idx >= 0; idx-- {
		*parts = append(*parts, newestFirst[idx])
	}
}

func collectRoleConversation(messages gjson.Result, parts *[]string, images *[]string, budget *contentModerationExtractionBudget) string {
	array := boundedTailResults(messages, maxContentModerationExtractionTurns, budget)
	if len(array) == 0 {
		return ""
	}
	return collectResponsesItems(array, parts, images, budget)
}

func collectAnthropicConversation(messages gjson.Result, parts *[]string, images *[]string, budget *contentModerationExtractionBudget) string {
	array := boundedTailResults(messages, maxContentModerationExtractionTurns, budget)
	if len(array) == 0 {
		return ""
	}
	return collectResponsesItems(array, parts, images, budget)
}

type responsesModerationItemKind int

const (
	responsesModerationItemIgnored responsesModerationItemKind = iota
	responsesModerationItemUser
	responsesModerationItemAssistant
	responsesModerationItemClientSystem
	responsesModerationItemClientDeveloper
	responsesModerationItemToolCall
	responsesModerationItemToolOutput
)

func isSupportedResponsesInput(input gjson.Result) bool {
	return input.Type == gjson.String || input.IsObject() || input.IsArray()
}

func collectResponsesConversation(input gjson.Result, parts *[]string, images *[]string, budget *contentModerationExtractionBudget) string {
	switch {
	case input.Type == gjson.String:
		var turnParts []string
		addModerationText(&turnParts, input.String())
		return appendModerationTurn(parts, "user", turnParts)
	case input.IsObject():
		if nested := input.Get("input"); isSupportedResponsesInput(nested) {
			return collectResponsesConversation(nested, parts, images, budget)
		}
		if nested := input.Get("messages"); nested.IsArray() {
			return collectResponsesConversation(nested, parts, images, budget)
		}
		return collectResponsesItems([]gjson.Result{input}, parts, images, budget)
	case input.IsArray():
		return collectResponsesItems(boundedTailResults(input, maxContentModerationExtractionTurns, budget), parts, images, budget)
	}
	return ""
}

func collectResponsesItems(items []gjson.Result, parts *[]string, images *[]string, budget *contentModerationExtractionBudget) string {
	if len(items) == 0 {
		return ""
	}
	latestMeaningful := -1
	latestKind := responsesModerationItemIgnored
	for idx := len(items) - 1; idx >= 0; idx-- {
		kind := classifyResponsesModerationItem(items[idx])
		if kind != responsesModerationItemIgnored {
			latestMeaningful = idx
			latestKind = kind
			break
		}
	}
	if latestMeaningful < 0 {
		return ""
	}

	latestUserIndex := -1
	for idx := latestMeaningful; idx >= 0; idx-- {
		if classifyResponsesModerationItem(items[idx]) == responsesModerationItemUser {
			latestUserIndex = idx
			break
		}
	}

	latestBudget := &contentModerationExtractionBudget{}
	var latestActiveParts []string
	var latestActiveImages []string
	collectResponsesItem(items[latestMeaningful], latestKind, &latestActiveParts, &latestActiveImages, latestBudget)
	latestActive := moderationPartsText(latestActiveParts)
	mergeContentModerationExtractionBudget(budget, latestBudget)

	latestUser := ""
	var latestUserImages []string
	var latestUserParts []string
	if latestUserIndex >= 0 {
		userBudget := &contentModerationExtractionBudget{}
		collectResponsesItem(items[latestUserIndex], responsesModerationItemUser, &latestUserParts, &latestUserImages, userBudget)
		latestUser = moderationPartsText(latestUserParts)
		mergeContentModerationExtractionBudget(budget, userBudget)
	}

	newestFirst := make([]string, 0, latestMeaningful+1)
	for idx := latestMeaningful; idx >= 0; idx-- {
		if idx == latestMeaningful {
			appendResponsesModerationTurn(&newestFirst, latestKind, latestActiveParts)
			continue
		}
		if idx == latestUserIndex {
			appendResponsesModerationTurn(&newestFirst, responsesModerationItemUser, latestUserParts)
			continue
		}
		if budget.exhausted() {
			budget.truncated = true
			continue
		}
		item := items[idx]
		kind := classifyResponsesModerationItem(item)
		if kind == responsesModerationItemIgnored {
			continue
		}
		var turnParts []string
		var turnImages []string
		collectResponsesItem(item, kind, &turnParts, &turnImages, budget)
		appendResponsesModerationTurn(&newestFirst, kind, turnParts)
	}
	appendNewestModerationTurns(parts, newestFirst)
	*images = append(*images, latestActiveImages...)
	*images = append(*images, latestUserImages...)

	if latestKind != responsesModerationItemUser && latestActive != "" {
		if latestUser == "" || latestUser == latestActive {
			return latestActive
		}
		label := "ASSISTANT"
		switch latestKind {
		case responsesModerationItemClientSystem:
			label = "CLIENT_SYSTEM"
		case responsesModerationItemClientDeveloper:
			label = "CLIENT_DEVELOPER"
		case responsesModerationItemToolCall:
			label = "TOOL_CALL"
		case responsesModerationItemToolOutput:
			label = "TOOL"
		}
		return normalizeContentModerationText(latestUser + "\n[" + label + "]\n" + latestActive)
	}
	if latestUser != "" {
		return latestUser
	}
	return latestActive
}

func appendResponsesModerationTurn(parts *[]string, kind responsesModerationItemKind, turnParts []string) {
	switch kind {
	case responsesModerationItemUser:
		appendModerationTurn(parts, "user", turnParts)
	case responsesModerationItemAssistant:
		appendModerationTurn(parts, "assistant", turnParts)
	case responsesModerationItemClientSystem:
		appendModerationLabeledTurn(parts, "client_system", turnParts)
	case responsesModerationItemClientDeveloper:
		appendModerationLabeledTurn(parts, "client_developer", turnParts)
	case responsesModerationItemToolCall:
		appendModerationLabeledTurn(parts, "tool_call", turnParts)
	case responsesModerationItemToolOutput:
		appendModerationLabeledTurn(parts, "tool", turnParts)
	}
}

func classifyResponsesModerationItem(item gjson.Result) responsesModerationItemKind {
	if item.Type == gjson.String {
		return responsesModerationItemUser
	}
	if !item.IsObject() {
		return responsesModerationItemIgnored
	}
	typ := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
	if isResponsesNoiseItemType(typ) {
		return responsesModerationItemIgnored
	}
	switch {
	case isCodexToolCallContextItemType(typ), typ == "computer_call", typ == "shell_call", typ == "terminal":
		return responsesModerationItemToolCall
	case isCodexToolCallOutputItemType(typ), typ == "tool_output", typ == "computer_call_output", typ == "local_shell_call_output", typ == "shell_call_output", typ == "terminal_output":
		return responsesModerationItemToolOutput
	case typ == "codex_delegation":
		return responsesModerationItemClientDeveloper
	case typ == "reasoning", typ == "computer_initialize_state", typ == "computer_screenshot":
		return responsesModerationItemAssistant
	}
	role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
	switch role {
	case "user":
		return responsesModerationItemUser
	case "assistant":
		return responsesModerationItemAssistant
	case "system":
		return responsesModerationItemClientSystem
	case "developer":
		return responsesModerationItemClientDeveloper
	case "tool", "function":
		return responsesModerationItemToolOutput
	}
	if role != "" {
		return responsesModerationItemClientSystem
	}
	if typ == "input_text" || typ == "input_image" || typ == "message" || item.Get("text").Exists() || item.Get("content").Exists() {
		return responsesModerationItemUser
	}
	if typ != "" {
		return responsesModerationItemClientSystem
	}
	return responsesModerationItemIgnored
}

func collectResponsesItem(item gjson.Result, kind responsesModerationItemKind, parts *[]string, images *[]string, budget *contentModerationExtractionBudget) {
	if item.Type == gjson.String {
		addModerationText(parts, item.String())
		return
	}
	switch kind {
	case responsesModerationItemToolCall:
		addModerationText(parts, item.Get("name").String())
		collectStructuredModerationValue(item.Get("arguments"), parts, images, budget, 0)
		collectStructuredModerationValue(item.Get("input"), parts, images, budget, 0)
		collectStructuredModerationValue(item.Get("action"), parts, images, budget, 0)
		collectStructuredModerationValue(item.Get("command"), parts, images, budget, 0)
		collectStructuredModerationValue(item.Get("cmd"), parts, images, budget, 0)
	case responsesModerationItemToolOutput:
		collectStructuredModerationValue(item.Get("output"), parts, images, budget, 0)
		collectContentValueBounded(item.Get("content"), parts, images, budget, 0)
	case responsesModerationItemUser, responsesModerationItemAssistant, responsesModerationItemClientSystem, responsesModerationItemClientDeveloper:
		typ := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
		if typ == "input_text" || typ == "output_text" || typ == "input_image" || typ == "image" || typ == "image_url" {
			collectContentValueBounded(item, parts, images, budget, 0)
			return
		}
		collectContentValueBounded(item.Get("content"), parts, images, budget, 0)
		if item.Get("text").Exists() {
			collectContentValueBounded(item.Get("text"), parts, images, budget, 0)
		}
		if nested := item.Get("message"); nested.Exists() {
			collectContentValueBounded(nested, parts, images, budget, 0)
		}
		collectStructuredModerationValue(item.Get("tool_calls"), parts, images, budget, 0)
		collectStructuredModerationValue(item.Get("function_call"), parts, images, budget, 0)
		if typ != "" && typ != "message" && typ != "input_text" && typ != "output_text" && typ != "input_image" && typ != "image" && typ != "image_url" {
			collectStructuredModerationValue(item, parts, images, budget, 0)
		}
	}
}

func collectStructuredModerationValue(value gjson.Result, parts *[]string, images *[]string, budget *contentModerationExtractionBudget, depth int) {
	if !value.Exists() || !budget.visit(depth) {
		return
	}
	switch {
	case value.Type == gjson.String:
		addModerationText(parts, value.String())
	case value.Type == gjson.Number || value.Type == gjson.True || value.Type == gjson.False:
		addModerationText(parts, value.Raw)
	case value.IsArray():
		for _, item := range boundedTailResults(value, maxContentModerationExtractionContentItems, budget) {
			collectStructuredModerationValue(item, parts, images, budget, depth+1)
		}
	case value.IsObject():
		addModerationImage(images, value.Get("image_url.url").String())
		addModerationImage(images, value.Get("image_url").String())
		addModerationImage(images, value.Get("image").String())
		addModerationImage(images, value.Get("screenshot").String())
		addModerationImage(images, value.Get("url").String())
		addModerationImageData(images, value.Get("media_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mime_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mimeType").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("media_type").String(), value.Get("base64").String())
		addModerationImageData(images, value.Get("mime_type").String(), value.Get("base64").String())
		addModerationImageData(images, value.Get("mimeType").String(), value.Get("base64").String())
		binaryData := value.Get("data").Type == gjson.String && (value.Get("media_type").Type == gjson.String ||
			value.Get("mime_type").Type == gjson.String ||
			looksLikeBase64Payload(value.Get("data").String()))
		value.ForEach(func(key, item gjson.Result) bool {
			if !budget.visit(depth + 1) {
				return false
			}
			field := strings.ToLower(strings.TrimSpace(key.String()))
			switch field {
			case "base64", "image", "image_url", "screenshot", "media_type", "mime_type", "mimetype", "type", "role", "id", "call_id":
				return true
			case "data":
				if binaryData {
					return true
				}
			}
			addModerationText(parts, key.String())
			collectStructuredModerationValue(item, parts, images, budget, depth+1)
			return !budget.exhausted()
		})
	}
}

func looksLikeBase64Payload(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 128 || len(value)%4 != 0 {
		return false
	}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '+' || ch == '/' || ch == '=' || ch == '\r' || ch == '\n' {
			continue
		}
		return false
	}
	return true
}

func isResponsesNoiseItemType(typ string) bool {
	return typ == "item_reference"
}

func collectGeminiConversation(contents gjson.Result, parts *[]string, images *[]string, budget *contentModerationExtractionBudget) string {
	array := boundedTailResults(contents, maxContentModerationExtractionTurns, budget)
	if len(array) == 0 {
		return ""
	}
	var latestParts []string
	var latestImages []string
	latestBudget := &contentModerationExtractionBudget{}
	collectGeminiParts(array[len(array)-1], &latestParts, &latestImages, latestBudget)
	mergeContentModerationExtractionBudget(budget, latestBudget)
	current := moderationPartsText(latestParts)
	if current == "" && len(latestImages) == 0 {
		return ""
	}
	newestFirst := make([]string, 0, len(array))
	for idx := len(array) - 1; idx >= 0; idx-- {
		if budget.exhausted() {
			budget.truncated = true
			break
		}
		content := array[idx]
		role := strings.ToLower(strings.TrimSpace(content.Get("role").String()))
		if role == "model" {
			role = "assistant"
		}
		if role == "" {
			role = "user"
		}
		var turnParts []string
		var turnImages []string
		collectGeminiParts(content, &turnParts, &turnImages, budget)
		var formatted []string
		if role == "user" || role == "assistant" {
			appendModerationTurn(&formatted, role, turnParts)
		} else {
			appendModerationLabeledTurn(&formatted, "client_role", append([]string{"role: " + role}, turnParts...))
		}
		if len(formatted) > 0 {
			newestFirst = append(newestFirst, formatted[0])
		}
	}
	appendNewestModerationTurns(parts, newestFirst)
	*images = append(*images, latestImages...)
	return current
}

func collectGeminiParts(content gjson.Result, parts *[]string, images *[]string, budget *contentModerationExtractionBudget) {
	arr := content.Get("parts")
	if !arr.IsArray() {
		return
	}
	for _, part := range boundedTailResults(arr, maxContentModerationExtractionContentItems, budget) {
		addModerationText(parts, part.Get("text").String())
		addGeminiModerationImage(images, part)
		collectStructuredModerationValue(part.Get("functionCall"), parts, images, budget, 0)
		collectStructuredModerationValue(part.Get("function_call"), parts, images, budget, 0)
		collectStructuredModerationValue(part.Get("functionResponse"), parts, images, budget, 0)
		collectStructuredModerationValue(part.Get("function_response"), parts, images, budget, 0)
		if !part.Get("text").Exists() &&
			!part.Get("inline_data").Exists() && !part.Get("inlineData").Exists() &&
			!part.Get("file_data").Exists() && !part.Get("fileData").Exists() &&
			!part.Get("functionCall").Exists() && !part.Get("function_call").Exists() &&
			!part.Get("functionResponse").Exists() && !part.Get("function_response").Exists() {
			collectStructuredModerationValue(part, parts, images, budget, 0)
		}
	}
}

func collectContentValue(value gjson.Result, parts *[]string, images *[]string) {
	collectContentValueBounded(value, parts, images, &contentModerationExtractionBudget{}, 0)
}

func collectContentValueBounded(value gjson.Result, parts *[]string, images *[]string, budget *contentModerationExtractionBudget, depth int) {
	if !value.Exists() || !budget.visit(depth) {
		return
	}
	switch {
	case value.Type == gjson.String:
		addModerationText(parts, value.String())
	case value.IsArray():
		for _, item := range boundedTailResults(value, maxContentModerationExtractionContentItems, budget) {
			collectContentValueBounded(item, parts, images, budget, depth+1)
		}
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		if isResponsesNoiseItemType(typ) {
			return
		}
		addModerationImage(images, value.Get("image_url.url").String())
		addModerationImage(images, value.Get("image_url").String())
		addModerationImage(images, value.Get("url").String())
		addModerationImageData(images, value.Get("source.media_type").String(), value.Get("source.data").String())
		addModerationImageData(images, value.Get("source.mediaType").String(), value.Get("source.data").String())
		addModerationImageData(images, value.Get("media_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mime_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mimeType").String(), value.Get("data").String())
		addModerationImage(images, value.Get("source.data").String())
		addModerationImage(images, value.Get("data").String())
		addModerationImage(images, value.Get("base64").String())
		switch typ {
		case "", "text", "input_text", "output_text", "message":
			if value.Get("text").Exists() {
				addModerationText(parts, value.Get("text").String())
			}
			if value.Get("content").Exists() {
				collectContentValueBounded(value.Get("content"), parts, images, budget, depth+1)
			}
		case "tool_use", "function_call", "tool_call", "tool_search_call", "custom_tool_call", "mcp_tool_call", "computer_call", "local_shell_call", "shell_call", "terminal":
			addModerationText(parts, value.Get("name").String())
			collectStructuredModerationValue(value.Get("arguments"), parts, images, budget, depth+1)
			collectStructuredModerationValue(value.Get("input"), parts, images, budget, depth+1)
			collectStructuredModerationValue(value.Get("action"), parts, images, budget, depth+1)
			collectStructuredModerationValue(value.Get("command"), parts, images, budget, depth+1)
			collectStructuredModerationValue(value.Get("cmd"), parts, images, budget, depth+1)
		case "tool_result", "function_call_output", "tool_output", "tool_search_output", "custom_tool_call_output", "mcp_tool_call_output", "computer_call_output", "local_shell_call_output", "shell_call_output", "terminal_output":
			collectStructuredModerationValue(value.Get("output"), parts, images, budget, depth+1)
			collectContentValueBounded(value.Get("content"), parts, images, budget, depth+1)
		case "codex_delegation", "reasoning", "computer_initialize_state", "computer_screenshot":
			collectStructuredModerationValue(value, parts, images, budget, depth+1)
		case "image_url", "input_image", "image":
		default:
			collectStructuredModerationValue(value, parts, images, budget, depth+1)
		}
	}
}

func boundedTailResults(value gjson.Result, limit int, budget *contentModerationExtractionBudget) []gjson.Result {
	if !value.IsArray() || limit <= 0 {
		return nil
	}
	buffer := make([]gjson.Result, limit)
	count := 0
	value.ForEach(func(_, item gjson.Result) bool {
		buffer[count%limit] = item
		count++
		return true
	})
	if count == 0 {
		return nil
	}
	if count <= limit {
		return append([]gjson.Result(nil), buffer[:count]...)
	}
	if budget != nil {
		budget.truncated = true
	}
	out := make([]gjson.Result, limit)
	start := count % limit
	for idx := 0; idx < limit; idx++ {
		out[idx] = buffer[(start+idx)%limit]
	}
	return out
}

func addGeminiModerationImage(images *[]string, part gjson.Result) {
	if inlineData := part.Get("inline_data"); inlineData.IsObject() {
		mimeType := strings.TrimSpace(inlineData.Get("mime_type").String())
		data := strings.TrimSpace(inlineData.Get("data").String())
		if mimeType != "" && data != "" {
			addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
		}
	}
	if inlineData := part.Get("inlineData"); inlineData.IsObject() {
		mimeType := strings.TrimSpace(inlineData.Get("mimeType").String())
		data := strings.TrimSpace(inlineData.Get("data").String())
		if mimeType != "" && data != "" {
			addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
		}
	}
	addModerationImage(images, part.Get("file_data.file_uri").String())
	addModerationImage(images, part.Get("fileData.fileUri").String())
}

func hasContentModerationImageField(root gjson.Result) bool {
	for _, field := range []string{"image", "images", "mask"} {
		if root.Get(field).Exists() {
			return true
		}
	}
	return false
}

func collectImageFields(root gjson.Result, images *[]string, budget *contentModerationExtractionBudget) {
	for _, field := range []string{"image", "images", "mask"} {
		collectImageValue(root.Get(field), images, budget, 0)
	}
}

func collectImageValue(value gjson.Result, images *[]string, budget *contentModerationExtractionBudget, depth int) {
	if !value.Exists() || !budget.visit(depth) {
		return
	}
	switch {
	case value.Type == gjson.String:
		addModerationImage(images, value.String())
	case value.IsArray():
		for _, item := range boundedTailResults(value, maxContentModerationExtractionContentItems, budget) {
			collectImageValue(item, images, budget, depth+1)
		}
	case value.IsObject():
		var ignored []string
		collectContentValueBounded(value, &ignored, images, budget, depth+1)
	}
}

func addModerationImageData(images *[]string, mimeType string, data string) {
	mimeType = strings.TrimSpace(mimeType)
	data = strings.TrimSpace(data)
	if mimeType == "" || data == "" {
		return
	}
	addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
}

func addModerationImage(images *[]string, image string) {
	image = strings.TrimSpace(image)
	if image == "" {
		return
	}
	if strings.HasPrefix(image, "data:") || strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		*images = append(*images, image)
	}
}

func normalizeModerationImages(images []string) []string {
	out := make([]string, 0, len(images))
	seen := make(map[string]struct{}, len(images))
	for _, image := range images {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		if _, ok := seen[image]; ok {
			continue
		}
		seen[image] = struct{}{}
		out = append(out, image)
	}
	return out
}

func limitContentModerationImages(images []string) []string {
	if len(images) <= maxContentModerationInputImages {
		return images
	}
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(images))))
	if err != nil {
		return images[:maxContentModerationInputImages]
	}
	return []string{images[int(idx.Int64())]}
}

func addModerationText(parts *[]string, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	*parts = append(*parts, text)
}

func moderationPartsText(parts []string) string {
	return normalizeContentModerationText(strings.Join(parts, "\n"))
}

func normalizeContentModerationText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func trimModerationContext(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	marker := []rune("\n\n[CONTEXT OMITTED]\n\n")
	if maxRunes <= len(marker)+2 {
		return string(runes[len(runes)-maxRunes:])
	}
	available := maxRunes - len(marker)
	headSize := available / 5
	tailSize := available - headSize
	return string(runes[:headSize]) + string(marker) + string(runes[len(runes)-tailSize:])
}
