package openai_compat

import "testing"

func TestResolvePromptCacheModeDefaultsOff(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  PromptCacheMode
	}{
		{name: "nil", want: PromptCacheModeOff},
		{name: "missing", extra: map[string]any{}, want: PromptCacheModeOff},
		{name: "invalid", extra: map[string]any{ExtraKeyPromptCacheMode: "enabled"}, want: PromptCacheModeOff},
		{name: "wrong type", extra: map[string]any{ExtraKeyPromptCacheMode: true}, want: PromptCacheModeOff},
		{name: "key only", extra: map[string]any{ExtraKeyPromptCacheMode: "key_only"}, want: PromptCacheModeKeyOnly},
		{name: "explicit", extra: map[string]any{ExtraKeyPromptCacheMode: "gpt56_explicit"}, want: PromptCacheModeGPT56Explicit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolvePromptCacheMode(test.extra); got != test.want {
				t.Fatalf("ResolvePromptCacheMode() = %q, want %q", got, test.want)
			}
		})
	}
}
