import { describe, expect, it } from 'vitest'
import { buildRikkaPayload, normalizeRikkaBaseUrl, parseRikkaModels } from './config'

describe('RikkaHub Responses contract', () => {
  it('normalizes HTTPS roots and prefixed v1 bases without duplicating the version', () => {
    expect(normalizeRikkaBaseUrl('https://example.com/')).toBe('https://example.com/v1')
    expect(normalizeRikkaBaseUrl('https://example.com/proxy/v1/')).toBe('https://example.com/proxy/v1')
    for (const url of ['http://example.com', 'https://user:pass@example.com', 'https://example.com/?key=secret', 'https://example.com/#key', 'https://example.com/v1/responses']) {
      expect(() => normalizeRikkaBaseUrl(url)).toThrow()
    }
  })

  it('encodes UTF-8 names and nested Responses reasoning without Chat parameters', () => {
    const encoded = buildRikkaPayload('https://example.com/v1', 'test-private-key', { id: '模型-测试', vision: true, effort: 'low' })
    expect(encoded.startsWith('ai-provider:v1:')).toBe(true)
    const bytes = Uint8Array.from(atob(encoded.slice('ai-provider:v1:'.length)), c => c.charCodeAt(0))
    const data = JSON.parse(new TextDecoder().decode(bytes))
    expect(data).toMatchObject({ apiKey: 'test-private-key', useResponseApi: true, responsesPath: '/responses' })
    expect(data.models[0]).toMatchObject({ modelId: '模型-测试', inputModalities: ['TEXT', 'IMAGE'], customBodies: [{ key: 'reasoning', value: { effort: 'low', summary: 'auto' } }] })
    expect(JSON.stringify(data)).not.toContain('reasoning_effort')
    expect(data).not.toHaveProperty('chatCompletionsPath')
  })

  it('uses catalog permissions and capabilities instead of assuming vision or reasoning', () => {
    expect(parseRikkaModels(JSON.stringify({ models: [
      { slug: 'disabled', supported_in_api: false },
      { slug: 'plain' }, { slug: 'plain' },
      { slug: 'vision', input_modalities: ['text', 'image'], default_reasoning_level: 'high', supported_reasoning_levels: [{ effort: 'low' }, { effort: 'high' }] },
      { slug: 'high-only', supported_reasoning_levels: [{ effort: 'high' }] }
    ] }))).toEqual([
      { id: 'plain', vision: false, effort: null },
      { id: 'vision', vision: true, effort: 'low' },
      { id: 'high-only', vision: false, effort: 'high' }
    ])
  })
})
