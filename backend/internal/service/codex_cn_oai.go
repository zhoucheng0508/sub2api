package service

import (
	"encoding/json"
	"strings"
)

// This deployment's dedicated group opts into client-side capability controls.
// Do not infer this policy from the OpenAI platform or a model-name prefix.
func isCNOAIGroup(group *Group) bool {
	return group != nil && group.Platform == PlatformOpenAI &&
		strings.EqualFold(strings.Join(strings.Fields(group.Name), ""), "国模OAI")
}

// Operator-requested permissive controls, not verified upstream capabilities.
// Apply to every visible model in this group so new models cannot fall back to
// none-only/text-only local gates. Group membership/allowlists are unchanged.
var cnOAICodexEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "auto"}

func applyCNOAICodexCatalog(body []byte, group *Group) ([]byte, error) {
	if !isCNOAIGroup(group) {
		return body, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	var models []map[string]json.RawMessage
	if err := json.Unmarshal(envelope["models"], &models); err != nil {
		return nil, err
	}
	for _, model := range models {
		var slug string
		if json.Unmarshal(model["slug"], &slug) != nil || strings.TrimSpace(slug) == "" {
			continue
		}
		levels := make([]configuredCodexReasoningLevel, 0, len(cnOAICodexEfforts))
		for _, effort := range cnOAICodexEfforts {
			levels = append(levels, configuredCodexReasoningLevel{Effort: effort, Description: configuredCodexReasoningLevelDescription(effort)})
		}
		model["supported_reasoning_levels"], _ = json.Marshal(levels)
		model["default_reasoning_level"] = json.RawMessage(`"medium"`)
		model["input_modalities"] = json.RawMessage(`["text","image"]`)
		model["supports_image_detail_original"] = json.RawMessage(`true`)
	}
	envelope["models"], _ = json.Marshal(models)
	return json.Marshal(envelope)
}
