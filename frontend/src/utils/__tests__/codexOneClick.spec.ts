import { describe, expect, it } from 'vitest'
import {
  buildCodexConfigFiles,
  buildCodexSetupScript,
  getCodexSetupFilename,
  getQuickConnectIneligibilityReason,
  isCodexOneClickEligible
} from '@/utils/codexOneClick'

describe('codexOneClick', () => {
  const usableKey = { status: 'active', key: 'sk-openai', group_id: 1, group: { platform: 'openai', status: 'active' } }

  it('requires a usable active key and assigned group', () => {
    expect(isCodexOneClickEligible(usableKey)).toBe(true)
    expect(isCodexOneClickEligible({ ...usableKey, group: { platform: 'anthropic' } })).toBe(true)
    expect(getQuickConnectIneligibilityReason({ ...usableKey, status: 'inactive' })).toBe('inactive')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, key: '' })).toBe('emptyKey')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, key: '   ' })).toBe('emptyKey')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, key: null })).toBe('emptyKey')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, group_id: null })).toBe('noGroup')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, group: undefined })).toBe('noGroup')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, group: { platform: 'openai', status: 'inactive' } })).toBe('inactiveGroup')
  })

  it('rejects expired or exhausted keys even when their stored status is still active', () => {
    expect(getQuickConnectIneligibilityReason({ ...usableKey, status: 'expired' })).toBe('expired')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, expires_at: new Date(Date.now() - 1_000).toISOString() })).toBe('expired')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, expires_at: new Date(Date.now() + 60_000).toISOString() })).toBeNull()
    expect(getQuickConnectIneligibilityReason({ ...usableKey, status: 'quota_exhausted' })).toBe('quotaExhausted')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, quota: 10, quota_used: 10 })).toBe('quotaExhausted')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, quota: 10, quota_used: 12 })).toBe('quotaExhausted')
    expect(getQuickConnectIneligibilityReason({ ...usableKey, quota: 0, quota_used: 12 })).toBeNull()
  })

  it('reserves image-only keys for the image scene', () => {
    const imageKey = { ...usableKey, group: { ...usableKey.group, image_only: true } }
    expect(isCodexOneClickEligible(imageKey)).toBe(false)
    expect(getQuickConnectIneligibilityReason(imageKey)).toBe('imageOnly')
    expect(getQuickConnectIneligibilityReason(imageKey, 'image')).toBeNull()
    expect(getQuickConnectIneligibilityReason({ ...imageKey, status: 'inactive' }, 'image')).toBe('inactive')
  })

  it('shares the established Codex configuration and supports API Key mode', () => {
    const [config, auth] = buildCodexConfigFiles(
      'https://api.example.com/with"quote/',
      'sk-secret',
      'api-key'
    )

    expect(config.content).toContain('model = "gpt-5.6"')
    expect(config.content).toContain('base_url = "https://api.example.com/with\\"quote/v1"')
    expect(config.content).toContain('requires_openai_auth = false')
    expect(config.content).toContain('x-openai-actor-authorization')
    expect(JSON.parse(auth.content)).toEqual({ OPENAI_API_KEY: 'sk-secret' })
  })

  it.each([
    'https://ai.vote520.com',
    'https://ai.vote520.com/',
    'https://ai.vote520.com/v1',
    'https://ai.vote520.com/v1/'
  ])('normalizes Codex base URL %s to exactly one /v1 suffix', (baseUrl) => {
    const [config] = buildCodexConfigFiles(baseUrl, 'sk-secret')
    expect(config.content).toContain('base_url = "https://ai.vote520.com/v1"')
    expect(config.content).not.toContain('/v1/v1')
  })

  it('generates OS-specific, reversible scripts without showing the plain key', () => {
    const mac = buildCodexSetupScript('macos', 'https://api.example.com', 'sk-secret')
    const linux = buildCodexSetupScript('linux', 'https://api.example.com', 'sk-secret')
    const windows = buildCodexSetupScript('windows', 'https://api.example.com', 'sk-secret')

    expect(mac).toContain('base64 -D')
    expect(linux).toContain('base64 --decode')
    expect(mac).toContain('mktemp -d')
    expect(mac).toContain('restore.sh')
    expect(windows).toContain('[IO.Path]::GetRandomFileName()')
    expect(windows).toContain('restore.ps1')
    expect([mac, linux, windows].every((script) => !script.includes('sk-secret'))).toBe(true)

    const encodedConfig = mac.match(/printf '%s' '([^']+)' \| base64 -D > "\$target_dir\/config\.toml\.tmp"/)?.[1]
    const encodedAuth = mac.match(/printf '%s' '([^']+)' \| base64 -D > "\$target_dir\/auth\.json\.tmp"/)?.[1]
    expect(encodedConfig).toBeTruthy()
    expect(encodedAuth).toBeTruthy()
    const decodedConfig = atob(encodedConfig!)
    expect(decodedConfig).toContain('model = "gpt-5.6"')
    expect(decodedConfig).toContain('review_model = "gpt-5.6"')
    expect(decodedConfig).toContain('wire_api = "responses"')
    expect(decodedConfig).toContain('requires_openai_auth = true')
    expect(decodedConfig).not.toContain('x-openai-actor-authorization')
    expect(JSON.parse(atob(encodedAuth!))).toEqual({ OPENAI_API_KEY: 'sk-secret' })

    const windowsPayloads = [...windows.matchAll(/\[Convert\]::FromBase64String\('([^']+)'\)/g)]
      .map((match) => atob(match[1]))
    expect(windowsPayloads).toHaveLength(2)
    expect(windowsPayloads[0]).toContain('model = "gpt-5.6"')
    expect(windowsPayloads[0]).toContain('review_model = "gpt-5.6"')
    expect(windowsPayloads[0]).toContain('wire_api = "responses"')
    expect(windowsPayloads[0]).toContain('requires_openai_auth = true')
    expect(JSON.parse(windowsPayloads[1])).toEqual({ OPENAI_API_KEY: 'sk-secret' })
  })

  it('uses conventional script extensions', () => {
    expect(getCodexSetupFilename('windows')).toMatch(/\.ps1$/)
    expect(getCodexSetupFilename('macos')).toMatch(/\.sh$/)
    expect(getCodexSetupFilename('linux')).toMatch(/\.sh$/)
  })
})
