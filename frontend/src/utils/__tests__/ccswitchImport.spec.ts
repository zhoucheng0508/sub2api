import { describe, expect, it } from 'vitest'
import {
  CC_SWITCH_APP_CATALOG,
  CC_SWITCH_IMPORTABLE_APP_TYPES,
  CLAUDE_CC_SWITCH_MODEL,
  GROK_CC_SWITCH_MODEL,
  OPENAI_CC_SWITCH_CODEX_MODEL,
  buildCcSwitchImportDeeplink,
  buildCcSwitchUsageUrl,
  isCcSwitchAppImportable,
  resolveCcSwitchImportConfig
} from '@/utils/ccswitchImport'
import type { GroupPlatform } from '@/types'

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
}

describe('ccswitchImport utils', () => {
  it('tracks all CC Switch managed apps in the same order as its settings UI', () => {
    expect(CC_SWITCH_APP_CATALOG.map((item) => item.id)).toEqual([
      'claude',
      'claude-desktop',
      'codex',
      'gemini',
      'grokbuild',
      'opencode',
      'openclaw',
      'hermes',
      'pi'
    ])
    expect(CC_SWITCH_IMPORTABLE_APP_TYPES).toHaveLength(7)
    expect(CC_SWITCH_APP_CATALOG.find((item) => item.id === 'claude-desktop')).toMatchObject({
      importable: false
    })
    expect(CC_SWITCH_APP_CATALOG.find((item) => item.id === 'pi')).toMatchObject({
      importable: false
    })
    expect(isCcSwitchAppImportable('claude')).toBe(true)
    expect(isCcSwitchAppImportable('claude-desktop')).toBe(false)
    expect(isCcSwitchAppImportable('pi')).toBe(false)
    expect(isCcSwitchAppImportable('unknown')).toBe(false)
  })

  it('defaults OpenAI CC Switch imports to the current Codex model', () => {
    expect(OPENAI_CC_SWITCH_CODEX_MODEL).toBe('gpt-5.6')
  })

  it('defaults Grok Build imports to the current Grok model', () => {
    expect(GROK_CC_SWITCH_MODEL).toBe('grok-4.5')
  })

  const baseInput = {
    baseUrl: 'https://api.example.com',
    providerName: 'Sub2API',
    apiKey: 'sk-test',
    usageScript: 'return true'
  }

  it.each([
    'https://ai.vote520.com',
    'https://ai.vote520.com/',
    'https://ai.vote520.com/v1',
    'https://ai.vote520.com/v1/',
    '  https://ai.vote520.com/v1/  '
  ])('builds one normalized /v1/usage URL for base URL %s', (baseUrl) => {
    expect(buildCcSwitchUsageUrl(baseUrl)).toBe('https://ai.vote520.com/v1/usage')
  })

  it.each([
    'https://ai.vote520.com',
    'https://ai.vote520.com/',
    'https://ai.vote520.com/v1',
    'https://ai.vote520.com/v1/'
  ])('adds the Codex model and one /v1 endpoint for OpenAI base URL %s', (baseUrl) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        platform: 'openai',
        clientType: 'claude'
      })
    )

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('homepage')).toBe(baseUrl)
    expect(params.get('endpoint')).toBe('https://ai.vote520.com/v1')
    expect(params.get('model')).toBe(OPENAI_CC_SWITCH_CODEX_MODEL)
    expect(atob(params.get('usageScript') || '')).toBe(baseInput.usageScript)
  })

  it('encodes non-ASCII usage scripts as UTF-8 before building the deeplink', () => {
    const usageScript = 'return { unit: "元", label: "可用额度" }'
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({
      ...baseInput,
      usageScript,
      platform: 'openai',
      clientType: 'claude'
    }))
    const decoded = new TextDecoder().decode(
      Uint8Array.from(atob(params.get('usageScript') || ''), (character) => character.charCodeAt(0))
    )
    expect(decoded).toBe(usageScript)
  })

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('imports Grok Build with one /v1 suffix for base URL %s', (baseUrl) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        platform: 'grok',
        clientType: 'claude'
      })
    )

    expect(params.get('app')).toBe('grokbuild')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('model')).toBe(GROK_CC_SWITCH_MODEL)
  })

  it.each([
    { platform: 'anthropic' as GroupPlatform, clientType: 'claude' as const, app: 'claude' },
    { platform: 'gemini' as GroupPlatform, clientType: 'gemini' as const, app: 'gemini' }
  ])('does not add a model parameter for $platform imports', ({ platform, clientType, app }) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform,
        clientType
      })
    )

    expect(params.get('app')).toBe(app)
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
  })

  it('keeps the legacy Claude/Gemini endpoint value when no explicit app is selected', () => {
    expect(resolveCcSwitchImportConfig('anthropic', 'claude', 'https://api.example.com/').endpoint)
      .toBe('https://api.example.com/')
    expect(resolveCcSwitchImportConfig('gemini', 'gemini', 'https://api.example.com/').endpoint)
      .toBe('https://api.example.com/')
  })

  it('keeps Antigravity imports on the selected client endpoint without a model parameter', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'antigravity',
        clientType: 'gemini'
      })
    )

    expect(params.get('app')).toBe('gemini')
    expect(params.get('endpoint')).toBe(`${baseInput.baseUrl}/antigravity`)
    expect(params.has('model')).toBe(false)
  })

  it.each([
    {
      app: 'claude' as const,
      platform: 'anthropic' as GroupPlatform,
      wireApp: 'claude',
      endpoint: 'https://api.example.com',
      model: CLAUDE_CC_SWITCH_MODEL
    },
    {
      app: 'codex' as const,
      platform: 'openai' as GroupPlatform,
      wireApp: 'codex',
      endpoint: 'https://api.example.com/v1',
      model: OPENAI_CC_SWITCH_CODEX_MODEL
    },
    {
      app: 'gemini' as const,
      platform: 'gemini' as GroupPlatform,
      wireApp: 'gemini',
      endpoint: 'https://api.example.com',
      model: undefined
    },
    {
      app: 'grokbuild' as const,
      platform: 'grok' as GroupPlatform,
      wireApp: 'grokbuild',
      endpoint: 'https://api.example.com/v1',
      model: GROK_CC_SWITCH_MODEL
    },
    {
      app: 'opencode' as const,
      platform: 'openai' as GroupPlatform,
      wireApp: 'opencode',
      endpoint: 'https://api.example.com/v1',
      model: OPENAI_CC_SWITCH_CODEX_MODEL
    },
    {
      app: 'openclaw' as const,
      platform: 'grok' as GroupPlatform,
      wireApp: 'openclaw',
      endpoint: 'https://api.example.com/v1',
      model: GROK_CC_SWITCH_MODEL
    },
    {
      app: 'hermes' as const,
      platform: 'anthropic' as GroupPlatform,
      wireApp: 'hermes',
      endpoint: 'https://api.example.com/v1',
      model: OPENAI_CC_SWITCH_CODEX_MODEL
    }
  ])('resolves explicit $app app IDs to a valid provider mapping', ({ app, platform, wireApp, endpoint, model }) => {
    const config = resolveCcSwitchImportConfig(platform, 'claude', 'https://api.example.com/', app)

    expect(config).toMatchObject({
      app: wireApp,
      endpoint,
      importable: true,
      requestedApp: app
    })
    if (model) {
      expect(config.model).toBe(model)
    } else {
      expect(config.model).toBeUndefined()
    }

    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({
      ...baseInput,
      platform,
      clientType: 'claude',
      app
    }))
    expect(params.get('app')).toBe(wireApp)
    expect(params.get('endpoint')).toBe(endpoint)
    expect(params.get('model') ?? undefined).toBe(model)
  })

  it('marks Pi as non-importable and refuses to build a provider deeplink', () => {
    const config = resolveCcSwitchImportConfig(
      'openai',
      'claude',
      'https://api.example.com',
      'pi'
    )
    expect(config.importable).toBe(false)
    expect(config.requestedApp).toBe('pi')
    expect(config.reason).toContain('Pi')

    expect(() => buildCcSwitchImportDeeplink({
      ...baseInput,
      platform: 'openai',
      clientType: 'claude',
      app: 'pi'
    })).toThrow(/Pi/)
  })

  it('allows explicit model overrides for additive app imports', () => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({
      ...baseInput,
      platform: 'anthropic',
      clientType: 'claude',
      app: 'opencode',
      model: 'claude-sonnet-5'
    }))

    expect(params.get('app')).toBe('opencode')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('model')).toBe('claude-sonnet-5')
  })

  it('refuses a Claude Desktop provider deeplink because CC Switch does not accept that app id', () => {
    expect(() => buildCcSwitchImportDeeplink({
      ...baseInput,
      platform: 'anthropic',
      clientType: 'claude',
      app: 'claude-desktop'
    })).toThrow(/Claude Desktop/)
  })
})
