package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCNOAICatalogGroupIsolation(t *testing.T) {
	ids := []string{"glm-5.3", "kimi-k2.5", "deepseek-v4-flash", "deepseek-v4.1-flash", "unrecognized-model"}
	original, err := BuildCodexModelsManifest(ids)
	require.NoError(t, err)
	for _, group := range []*Group{nil, {Name: "Other", Platform: PlatformOpenAI}, {Name: "国模 OAI", Platform: PlatformComposite}, {Name: "国模 OAI backup", Platform: PlatformOpenAI}} {
		got, err := applyCNOAICodexCatalog(original, group)
		require.NoError(t, err)
		require.Equal(t, original, got)
	}
	group := &Group{Name: " 国模 Oai ", Platform: PlatformOpenAI}
	body, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, ids, nil, group, nil, true)
	require.NoError(t, err)
	var catalog struct {
		Models []configuredCodexModelDescriptor `json:"models"`
	}
	require.NoError(t, json.Unmarshal(body, &catalog))
	require.Len(t, catalog.Models, len(ids)) // No added models bypass the group allowlist.
	for i, model := range catalog.Models {
		require.Equal(t, ids[i], model.Slug)
		require.Equal(t, []string{"text", "image"}, model.InputModalities)
		require.Equal(t, "medium", *model.DefaultReasoningLevel)
		var efforts []string
		for _, level := range model.SupportedReasoningLevels {
			efforts = append(efforts, level.Effort)
		}
		require.Equal(t, []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "auto"}, efforts)
	}
}

func TestCNOAICatalogPreservesUnrelatedMetadata(t *testing.T) {
	body := []byte(`{"other":"preserved","models":[{"slug":"glm-5.3","supports_search_tool":true,"model_messages":{"instructions_template":"keep"},"input_modalities":["text"]}]}`)
	got, err := applyCNOAICodexCatalog(body, &Group{Name: "国模OAI", Platform: PlatformOpenAI})
	require.NoError(t, err)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got, &envelope))
	require.JSONEq(t, `"preserved"`, string(envelope["other"]))
	var models []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope["models"], &models))
	require.JSONEq(t, `true`, string(models[0]["supports_search_tool"]))
	require.JSONEq(t, `{"instructions_template":"keep"}`, string(models[0]["model_messages"]))
	require.JSONEq(t, `true`, string(models[0]["supports_image_detail_original"]))
	_, err = applyCNOAICodexCatalog([]byte(`invalid`), &Group{Name: "国模OAI", Platform: PlatformOpenAI})
	require.Error(t, err)
}
