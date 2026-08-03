package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/tidwall/gjson"
)

func ExtractContentModerationText(protocol string, body []byte) string {
	return ExtractContentModerationInput(protocol, body).Text
}

func ExtractContentModerationInput(protocol string, body []byte) ContentModerationInput {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ContentModerationInput{}
	}
	var parts []string
	var images []string
	var currentText string
	switch protocol {
	case ContentModerationProtocolAnthropicMessages:
		currentText = collectAnthropicConversation(gjson.GetBytes(body, "messages"), &parts, &images)
	case ContentModerationProtocolOpenAIChat:
		currentText = collectRoleConversation(gjson.GetBytes(body, "messages"), &parts, &images)
	case ContentModerationProtocolOpenAIResponses:
		currentText = collectResponsesConversation(gjson.GetBytes(body, "input"), &parts, &images)
	case ContentModerationProtocolGemini:
		currentText = collectGeminiConversation(gjson.GetBytes(body, "contents"), &parts, &images)
	case ContentModerationProtocolOpenAIImages:
		addModerationText(&parts, gjson.GetBytes(body, "prompt").String())
		currentText = normalizeContentModerationText(strings.Join(parts, "\n"))
		collectContentValue(gjson.GetBytes(body, "images"), &parts, &images)
	default:
		currentText = collectResponsesConversation(gjson.GetBytes(body, "input"), &parts, &images)
		if currentText == "" {
			currentText = collectRoleConversation(gjson.GetBytes(body, "messages"), &parts, &images)
		}
		if currentText == "" {
			currentText = collectGeminiConversation(gjson.GetBytes(body, "contents"), &parts, &images)
		}
	}
	out := ContentModerationInput{
		Text:        strings.TrimSpace(strings.Join(parts, "\n\n")),
		CurrentText: currentText,
		Images:      normalizeModerationImages(images),
	}
	out.Normalize()
	return out
}

func appendModerationTurn(parts *[]string, role string, turnParts []string) string {
	text := normalizeContentModerationText(strings.Join(turnParts, "\n"))
	if text == "" {
		return ""
	}
	*parts = append(*parts, fmt.Sprintf("[%s]\n%s", strings.ToUpper(role), text))
	return text
}

func collectRoleConversation(messages gjson.Result, parts *[]string, images *[]string) string {
	if !messages.IsArray() {
		return ""
	}
	array := messages.Array()
	if len(array) == 0 || strings.ToLower(strings.TrimSpace(array[len(array)-1].Get("role").String())) != "user" {
		return ""
	}
	var latestParts []string
	var latestImages []string
	collectContentValue(array[len(array)-1].Get("content"), &latestParts, &latestImages)
	current := normalizeContentModerationText(strings.Join(latestParts, "\n"))
	if current == "" && len(latestImages) == 0 {
		return ""
	}
	for idx, message := range array {
		role := strings.ToLower(strings.TrimSpace(message.Get("role").String()))
		if role != "user" && role != "assistant" {
			continue
		}
		var turnParts []string
		var turnImages []string
		collectContentValue(message.Get("content"), &turnParts, &turnImages)
		appendModerationTurn(parts, role, turnParts)
		if idx == len(array)-1 {
			*images = append(*images, turnImages...)
		}
	}
	return current
}

func collectAnthropicConversation(messages gjson.Result, parts *[]string, images *[]string) string {
	if !messages.IsArray() {
		return ""
	}
	array := messages.Array()
	if len(array) == 0 || strings.ToLower(strings.TrimSpace(array[len(array)-1].Get("role").String())) != "user" {
		return ""
	}
	var latestParts []string
	var latestImages []string
	collectAnthropicUserContentValue(array[len(array)-1].Get("content"), &latestParts, &latestImages)
	current := normalizeContentModerationText(strings.Join(latestParts, "\n"))
	if current == "" && len(latestImages) == 0 {
		return ""
	}
	for idx, message := range array {
		role := strings.ToLower(strings.TrimSpace(message.Get("role").String()))
		if role != "user" && role != "assistant" {
			continue
		}
		var turnParts []string
		var turnImages []string
		if role == "user" {
			collectAnthropicUserContentValue(message.Get("content"), &turnParts, &turnImages)
		} else {
			collectContentValue(message.Get("content"), &turnParts, &turnImages)
		}
		appendModerationTurn(parts, role, turnParts)
		if idx == len(array)-1 {
			*images = append(*images, turnImages...)
		}
	}
	return current
}

func collectLastRoleMessage(messages gjson.Result, role string, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
	}
	array := messages.Array()
	if len(array) == 0 {
		return
	}
	last := array[len(array)-1]
	if strings.ToLower(strings.TrimSpace(last.Get("role").String())) != role {
		return
	}
	var candidate []string
	var candidateImages []string
	collectContentValue(last.Get("content"), &candidate, &candidateImages)
	if normalizeContentModerationText(strings.Join(candidate, "\n")) == "" && len(candidateImages) == 0 {
		return
	}
	*parts = append(*parts, candidate...)
	*images = append(*images, candidateImages...)
}

func collectLastAnthropicUserMessage(messages gjson.Result, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
	}
	array := messages.Array()
	if len(array) == 0 {
		return
	}
	last := array[len(array)-1]
	if strings.ToLower(strings.TrimSpace(last.Get("role").String())) != "user" {
		return
	}
	var candidate []string
	var candidateImages []string
	collectAnthropicUserContentValue(last.Get("content"), &candidate, &candidateImages)
	if normalizeContentModerationText(strings.Join(candidate, "\n")) == "" && len(candidateImages) == 0 {
		return
	}
	*parts = append(*parts, candidate...)
	*images = append(*images, candidateImages...)
}

func collectAnthropicUserContentValue(value gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		if !isAnthropicSystemReminderText(value.String()) {
			addModerationText(parts, value.String())
		}
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collectAnthropicUserContentValue(item, parts, images)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		switch typ {
		case "", "text", "input_text", "output_text", "message":
			if value.Get("text").Exists() && !isAnthropicSystemReminderText(value.Get("text").String()) {
				addModerationText(parts, value.Get("text").String())
			}
			if value.Get("content").Exists() {
				collectAnthropicUserContentValue(value.Get("content"), parts, images)
			}
		case "image_url", "input_image", "image":
			collectContentValue(value, parts, images)
		}
	}
}

func isAnthropicSystemReminderText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "<system-reminder>")
}

func collectLastResponsesInput(input gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !input.Exists():
		return
	case input.Type == gjson.String:
		addModerationText(parts, input.String())
	case input.IsArray():
		array := input.Array()
		if len(array) == 0 {
			return
		}
		last := array[len(array)-1]
		if !isResponsesUserTextItem(last) {
			return
		}
		collectContentValue(last.Get("content"), parts, images)
		if last.Get("type").String() == "input_text" || last.Get("text").Exists() {
			collectContentValue(last, parts, images)
		}
	case input.IsObject():
		if isResponsesUserTextItem(input) {
			collectContentValue(input.Get("content"), parts, images)
			if input.Get("type").String() == "input_text" || input.Get("text").Exists() {
				collectContentValue(input, parts, images)
			}
		}
	}
}

func collectResponsesConversation(input gjson.Result, parts *[]string, images *[]string) string {
	switch {
	case !input.Exists():
		return ""
	case input.Type == gjson.String:
		text := normalizeContentModerationText(input.String())
		if text != "" {
			*parts = append(*parts, "[USER]\n"+text)
		}
		return text
	case input.IsObject():
		if !isResponsesUserTextItem(input) {
			return ""
		}
		var turnParts []string
		collectResponsesItem(input, &turnParts, images)
		return appendModerationTurn(parts, "user", turnParts)
	case input.IsArray():
		array := input.Array()
		if len(array) == 0 || !isResponsesUserTextItem(array[len(array)-1]) {
			return ""
		}
		var latestParts []string
		var latestImages []string
		collectResponsesItem(array[len(array)-1], &latestParts, &latestImages)
		current := normalizeContentModerationText(strings.Join(latestParts, "\n"))
		if current == "" && len(latestImages) == 0 {
			return ""
		}
		for idx, item := range array {
			role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
			if role != "user" && role != "assistant" {
				continue
			}
			var turnParts []string
			var turnImages []string
			collectResponsesItem(item, &turnParts, &turnImages)
			appendModerationTurn(parts, role, turnParts)
			if idx == len(array)-1 {
				*images = append(*images, turnImages...)
			}
		}
		return current
	}
	return ""
}

func collectResponsesItem(item gjson.Result, parts *[]string, images *[]string) {
	collectContentValue(item.Get("content"), parts, images)
	if item.Get("type").String() == "input_text" || item.Get("type").String() == "output_text" || item.Get("text").Exists() {
		collectContentValue(item, parts, images)
	}
}

func isResponsesUserTextItem(item gjson.Result) bool {
	role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
	if role == "user" {
		return responseItemHasModerationText(item)
	}
	if role != "" {
		return false
	}
	return responseItemHasModerationText(item)
}

func responseItemHasModerationText(item gjson.Result) bool {
	var parts []string
	var images []string
	collectContentValue(item.Get("content"), &parts, &images)
	if item.Get("type").String() == "input_text" || item.Get("text").Exists() {
		collectContentValue(item, &parts, &images)
	}
	return normalizeContentModerationText(strings.Join(parts, "\n")) != "" || len(images) > 0
}

func collectLastGeminiContent(contents gjson.Result, parts *[]string, images *[]string) {
	if !contents.IsArray() {
		return
	}
	array := contents.Array()
	if len(array) == 0 {
		return
	}
	last := array[len(array)-1]
	role := strings.ToLower(strings.TrimSpace(last.Get("role").String()))
	if role != "" && role != "user" {
		return
	}
	var candidate []string
	var candidateImages []string
	if arr := last.Get("parts"); arr.IsArray() {
		arr.ForEach(func(_, part gjson.Result) bool {
			addModerationText(&candidate, part.Get("text").String())
			addGeminiModerationImage(&candidateImages, part)
			return true
		})
	}
	if normalizeContentModerationText(strings.Join(candidate, "\n")) == "" && len(candidateImages) == 0 {
		return
	}
	*parts = append(*parts, candidate...)
	*images = append(*images, candidateImages...)
}

func collectGeminiConversation(contents gjson.Result, parts *[]string, images *[]string) string {
	if !contents.IsArray() {
		return ""
	}
	array := contents.Array()
	if len(array) == 0 {
		return ""
	}
	lastRole := strings.ToLower(strings.TrimSpace(array[len(array)-1].Get("role").String()))
	if lastRole != "" && lastRole != "user" {
		return ""
	}
	var latestParts []string
	var latestImages []string
	collectGeminiParts(array[len(array)-1], &latestParts, &latestImages)
	current := normalizeContentModerationText(strings.Join(latestParts, "\n"))
	if current == "" && len(latestImages) == 0 {
		return ""
	}
	for idx, content := range array {
		role := strings.ToLower(strings.TrimSpace(content.Get("role").String()))
		if role == "model" {
			role = "assistant"
		}
		if role == "" {
			role = "user"
		}
		if role != "user" && role != "assistant" {
			continue
		}
		var turnParts []string
		var turnImages []string
		collectGeminiParts(content, &turnParts, &turnImages)
		appendModerationTurn(parts, role, turnParts)
		if idx == len(array)-1 {
			*images = append(*images, turnImages...)
		}
	}
	return current
}

func collectGeminiParts(content gjson.Result, parts *[]string, images *[]string) {
	if arr := content.Get("parts"); arr.IsArray() {
		arr.ForEach(func(_, part gjson.Result) bool {
			addModerationText(parts, part.Get("text").String())
			addGeminiModerationImage(images, part)
			return true
		})
	}
}

func collectContentValue(value gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		addModerationText(parts, value.String())
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collectContentValue(item, parts, images)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
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
				collectContentValue(value.Get("content"), parts, images)
			}
		case "image_url", "input_image", "image":
		}
	}
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
	if strings.Contains(text, "<system-reminder>") {
		return
	}
	*parts = append(*parts, text)
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
