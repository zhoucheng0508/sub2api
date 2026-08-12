package openai_compat

// PromptCacheMode controls account-level prompt-cache augmentation for
// OpenAI API-key accounts. Missing and invalid values intentionally normalize
// to off so upgrades cannot change existing third-party upstream behavior.
type PromptCacheMode string

const (
	PromptCacheModeOff           PromptCacheMode = "off"
	PromptCacheModeKeyOnly       PromptCacheMode = "key_only"
	PromptCacheModeGPT56Explicit PromptCacheMode = "gpt56_explicit"

	ExtraKeyPromptCacheMode = "openai_prompt_cache_mode"
)

func NormalizePromptCacheMode(value string) PromptCacheMode {
	switch PromptCacheMode(value) {
	case PromptCacheModeKeyOnly:
		return PromptCacheModeKeyOnly
	case PromptCacheModeGPT56Explicit:
		return PromptCacheModeGPT56Explicit
	default:
		return PromptCacheModeOff
	}
}

func ResolvePromptCacheMode(extra map[string]any) PromptCacheMode {
	if extra == nil {
		return PromptCacheModeOff
	}
	value, _ := extra[ExtraKeyPromptCacheMode].(string)
	return NormalizePromptCacheMode(value)
}
