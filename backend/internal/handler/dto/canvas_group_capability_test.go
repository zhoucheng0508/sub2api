package dto

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"strings"
	"testing"
)

func TestCanvasGroupCapabilityDoesNotExposeAdministratorAllowlist(t *testing.T) {
	for _, tt := range []struct {
		name    string
		enabled bool
		models  []string
		want    bool
	}{
		{"images", true, []string{"gpt-image-2", "gpt-image-2.5"}, true},
		{"mixed", true, []string{"gpt-image-2", "gpt-6-astra"}, false},
		{"disabled", false, []string{"gpt-image-2"}, false},
		{"empty", true, nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			group := &service.Group{Platform: service.PlatformOpenAI, ModelAllowlist: service.GroupModelAllowlist{Enabled: tt.enabled, Models: tt.models}}
			dto := GroupFromService(group)
			if dto.ImageOnly != tt.want {
				t.Fatalf("image-only classification = %v", dto.ImageOnly)
			}
			data, err := json.Marshal(dto)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "model_allowlist") {
				t.Fatal("administrator model rules leaked")
			}
		})
	}
}
