import type { GroupPlatform } from '@/types'

export const OPENAI_CC_SWITCH_CODEX_MODEL = 'gpt-5.6'
export const GROK_CC_SWITCH_MODEL = 'grok-4.5'
export const CLAUDE_CC_SWITCH_MODEL = 'claude-sonnet-5'

export type CcSwitchClientType = 'claude' | 'gemini'

/**
 * App identifiers understood by CC Switch's provider deeplink parser.
 *
 * CC Switch exposes nine managed app tabs today. `claude-desktop` is kept in
 * the catalog for parity with that UI, but its provider page does not
 * participate in the provider deeplink protocol. Pi is also represented as
 * non-importable because it must be configured from the Pi provider page.
 */
export type CcSwitchAppType =
  | 'claude'
  | 'claude-desktop'
  | 'codex'
  | 'gemini'
  | 'grokbuild'
  | 'opencode'
  | 'openclaw'
  | 'hermes'
  | 'pi'

export interface CcSwitchAppCatalogItem {
  id: CcSwitchAppType
  label: string
  icon: string
  importable: boolean
  aliasOf?: CcSwitchAppType
  reason?: string
}

/** Canonical order used by CC Switch's Settings > Home display. */
export const CC_SWITCH_APP_CATALOG: readonly CcSwitchAppCatalogItem[] = [
  { id: 'claude', label: 'Claude Code', icon: 'terminal', importable: true },
  {
    id: 'claude-desktop',
    label: 'Claude Desktop',
    icon: 'terminal',
    importable: false,
    reason: 'Claude Desktop providers must be added from the Claude Desktop provider page.'
  },
  { id: 'codex', label: 'Codex', icon: 'cpu', importable: true },
  { id: 'gemini', label: 'Gemini', icon: 'sparkles', importable: true },
  { id: 'grokbuild', label: 'Grok Build', icon: 'sparkles', importable: true },
  { id: 'opencode', label: 'OpenCode', icon: 'terminal', importable: true },
  { id: 'openclaw', label: 'OpenClaw', icon: 'terminal', importable: true },
  { id: 'hermes', label: 'Hermes', icon: 'terminal', importable: true },
  {
    id: 'pi',
    label: 'Pi',
    icon: 'terminal',
    importable: false,
    reason: 'Pi providers must be added from the Pi provider page.'
  }
]

/** Unique provider app ids accepted by `resource=provider` deeplinks. */
export const CC_SWITCH_IMPORTABLE_APP_TYPES = [
  'claude',
  'codex',
  'gemini',
  'grokbuild',
  'opencode',
  'openclaw',
  'hermes'
] as const satisfies readonly CcSwitchAppType[]

export function isCcSwitchAppImportable(app: string): app is CcSwitchAppType {
  return CC_SWITCH_APP_CATALOG.some((item) => item.id === app && item.importable)
}

export interface CcSwitchImportConfig {
  app: string
  endpoint: string
  model?: string
  importable: boolean
  reason?: string
  requestedApp?: CcSwitchAppType
  aliasOf?: CcSwitchAppType
}

export interface CcSwitchImportDeeplinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  clientType: CcSwitchClientType
  providerName: string
  apiKey: string
  usageScript: string
  /** Optional explicit CC Switch app; omitted for the legacy platform mapping. */
  app?: CcSwitchAppType | null
  /** Optional model override for app configurations that carry a model. */
  model?: string | null
}

export function normalizeV1Endpoint(baseUrl: string): string {
  const normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, '')
  return normalizedBaseUrl.endsWith('/v1') ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`
}

export function buildCcSwitchUsageUrl(baseUrl: string): string {
  return `${normalizeV1Endpoint(baseUrl)}/usage`
}

function normalizeBaseEndpoint(baseUrl: string): string {
  return baseUrl.trim().replace(/\/+$/, '')
}

function defaultAdditiveModel(platform: GroupPlatform | undefined | null): string {
  return platform === 'grok' ? GROK_CC_SWITCH_MODEL : OPENAI_CC_SWITCH_CODEX_MODEL
}

function resolveLegacyApp(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType
): CcSwitchAppType {
  switch (platform || 'anthropic') {
    case 'antigravity':
      return clientType === 'gemini' ? 'gemini' : 'claude'
    case 'openai':
      return 'codex'
    case 'gemini':
      return 'gemini'
    case 'grok':
      return 'grokbuild'
    default:
      return 'claude'
  }
}

function resolveCatalogItem(app: CcSwitchAppType | null | undefined): CcSwitchAppCatalogItem | undefined {
  if (!app) return undefined
  return CC_SWITCH_APP_CATALOG.find((item) => item.id === app)
}

function utf8ToBase64(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach((byte) => { binary += String.fromCharCode(byte) })
  return btoa(binary)
}

export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string,
  explicitApp?: CcSwitchAppType | null
): CcSwitchImportConfig {
  const requestedApp = explicitApp || resolveLegacyApp(platform, clientType)
  const catalogItem = resolveCatalogItem(requestedApp)
  const wireApp = catalogItem?.aliasOf || requestedApp
  const normalizedBase = normalizeBaseEndpoint(baseUrl)
  const preserveLegacyEndpoint = explicitApp == null

  if (!catalogItem) {
    return {
      app: wireApp,
      endpoint: normalizedBase,
      importable: false,
      requestedApp,
      reason: `Unsupported CC Switch app: ${requestedApp}`
    }
  }

  let endpoint = normalizedBase
  let model: string | undefined

  switch (wireApp) {
    case 'codex':
      endpoint = normalizeV1Endpoint(normalizedBase)
      model = OPENAI_CC_SWITCH_CODEX_MODEL
      break
    case 'grokbuild':
      endpoint = normalizeV1Endpoint(normalizedBase)
      model = GROK_CC_SWITCH_MODEL
      break
    case 'opencode':
    case 'openclaw':
    case 'hermes':
      endpoint = normalizeV1Endpoint(normalizedBase)
      model = defaultAdditiveModel(platform)
      break
    case 'claude':
      if (explicitApp) model = CLAUDE_CC_SWITCH_MODEL
      // Antigravity has a dedicated path for its Claude/Gemini clients.
      if (platform === 'antigravity') endpoint = `${normalizedBase}/antigravity`
      else if (preserveLegacyEndpoint) endpoint = baseUrl
      break
    case 'gemini':
      // Antigravity has a dedicated path for its Claude/Gemini clients.
      if (platform === 'antigravity') endpoint = `${normalizedBase}/antigravity`
      else if (preserveLegacyEndpoint) endpoint = baseUrl
      break
    case 'pi':
      // Kept for defensive completeness; the catalog currently marks Pi
      // non-importable and this branch should never produce a link.
      break
  }

  const config: CcSwitchImportConfig = {
    app: wireApp,
    endpoint,
    importable: catalogItem.importable,
    requestedApp
  }
  if (model) config.model = model
  if (catalogItem.aliasOf) config.aliasOf = catalogItem.aliasOf
  if (catalogItem.reason) config.reason = catalogItem.reason
  return config
}

export function buildCcSwitchImportDeeplink(input: CcSwitchImportDeeplinkInput): string {
  const config = resolveCcSwitchImportConfig(input.platform, input.clientType, input.baseUrl, input.app)
  if (!config.importable) {
    throw new Error(config.reason || `CC Switch app '${config.app}' does not support provider imports`)
  }

  const requestedModel = typeof input.model === 'string' ? input.model.trim() : ''
  const model = requestedModel || config.model
  const entries: [string, string][] = [
    ['resource', 'provider'],
    ['app', config.app],
    ['name', input.providerName],
    ['homepage', input.baseUrl],
    ['endpoint', config.endpoint],
    ['apiKey', input.apiKey],
    ['configFormat', 'json'],
    ['usageEnabled', 'true'],
    ['usageScript', utf8ToBase64(input.usageScript)],
    ['usageAutoInterval', '30']
  ]

  if (model) {
    entries.splice(2, 0, ['model', model])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}
