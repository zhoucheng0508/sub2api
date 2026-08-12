import { describe, expect, it } from 'vitest'
import {
  buildCodexConfigFiles,
  buildCodexSetupScript,
  getCodexSetupFilename,
  isCodexOneClickEligible
} from '@/utils/codexOneClick'

describe('codexOneClick', () => {
  it('enables one-click setup for every active row with a usable key', () => {
    expect(isCodexOneClickEligible({ status: 'active', key: 'sk-openai' })).toBe(true)
    expect(isCodexOneClickEligible({ status: 'active', key: 'sk-any-group' })).toBe(true)
    expect(isCodexOneClickEligible({ status: 'inactive', key: 'sk-inactive' })).toBe(false)
    expect(isCodexOneClickEligible({ status: 'active', key: '' })).toBe(false)
    expect(isCodexOneClickEligible({ status: 'active', key: '   ' })).toBe(false)
    expect(isCodexOneClickEligible({ status: 'active', key: null })).toBe(false)
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
