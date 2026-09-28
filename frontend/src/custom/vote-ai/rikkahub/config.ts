import { parseCodexCatalogModels, selectCodexConfigReasoningEffort } from '@/utils/codexCatalogConfig'

export interface RikkaModel {
  id: string
  vision: boolean
  effort: string | null
}

export function normalizeRikkaBaseUrl(value: string): string {
  const url = new URL(value || window.location.origin)
  if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash) {
    throw new Error('A clean HTTPS API base URL is required')
  }
  const path = url.pathname.replace(/\/+$/, '')
  if (/\/(responses|chat\/completions|models)$/i.test(path)) {
    throw new Error('Use the API base URL, not an endpoint')
  }
  url.pathname = /\/v1$/i.test(path) ? path : `${path}/v1`
  return url.toString().replace(/\/$/, '')
}

export function parseRikkaModels(content: string): RikkaModel[] {
  const seen = new Set<string>()
  return parseCodexCatalogModels(content).flatMap(model => {
    const metadata = model as typeof model & { input_modalities?: unknown; supported_in_api?: unknown }
    if (metadata.supported_in_api === false || seen.has(model.slug)) return []
    seen.add(model.slug)
    const levels = model.supported_reasoning_levels ?? []
    return [{
      id: model.slug,
      vision: Array.isArray(metadata.input_modalities) && metadata.input_modalities.includes('image'),
      effort: levels.some(level => level.effort === 'low') ? 'low' : selectCodexConfigReasoningEffort(model)
    }]
  })
}

/** RikkaHub 2.5.5 provider import contract. No key or payload is persisted. */
export function buildRikkaPayload(baseUrl: string, apiKey: string, model: RikkaModel): string {
  if (!apiKey.trim() || !model.id.trim()) throw new Error('Key and model are required')
  const provider = {
    type: 'openai',
    id: crypto.randomUUID(),
    enabled: true,
    name: `${new URL(baseUrl).hostname} (Responses)`,
    baseUrl: normalizeRikkaBaseUrl(baseUrl),
    apiKey,
    useResponseApi: true,
    responsesPath: '/responses',
    models: [{
      id: crypto.randomUUID(),
      modelId: model.id,
      displayName: model.id,
      type: 'CHAT',
      inputModalities: model.vision ? ['TEXT', 'IMAGE'] : ['TEXT'],
      outputModalities: ['TEXT'],
      abilities: model.effort ? ['REASONING'] : [],
      customBodies: model.effort
        ? [{ key: 'reasoning', value: { effort: model.effort, summary: 'auto' } }]
        : []
    }]
  }
  const bytes = new TextEncoder().encode(JSON.stringify(provider))
  return `ai-provider:v1:${btoa(Array.from(bytes, byte => String.fromCharCode(byte)).join(''))}`
}
