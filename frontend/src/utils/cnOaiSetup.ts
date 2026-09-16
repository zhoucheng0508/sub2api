import setupPowerShell from './cnOaiSetup.ps1?raw'
import { buildCodexModelsManifestUrl } from '@/api/codex'
import { normalizeV1Endpoint } from './ccswitchImport'

export function isCnOaiGroup(group?: { name?: string; platform?: string } | null): boolean {
  return group?.platform === 'openai' && group.name?.replace(/\s/g, '').toLowerCase() === '国模oai'
}

export interface CnOaiSetupInput {
  baseUrl: string
  apiKey: string
  providerName: string
  groupId: number
  model?: string
}

export const CN_OAI_SETUP_FILENAME = 'sub2api-cn-oai-setup.cmd'

export function buildCnOaiSetupScript(input: CnOaiSetupInput): string {
  const endpoint = normalizeV1Endpoint(input.baseUrl)
  const url = new URL(endpoint)
  if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
    throw new Error('Invalid Sub2API endpoint')
  }
  const payload = JSON.stringify({
    endpoint, manifestUrl: buildCodexModelsManifestUrl(input.baseUrl),
    apiKey: input.apiKey, providerName: `${input.providerName} · 国模 OAI`,
    groupId: input.groupId, model: input.model?.trim() || ''
  })
  const encoded = btoa(Array.from(new TextEncoder().encode(payload), b => String.fromCharCode(b)).join(''))
  // A short, fixed CMD launcher avoids the command-line size limit and never
  // interpolates credentials or user strings into executable shell syntax.
  return [
    '@echo off',
    'setlocal DisableDelayedExpansion',
    'set "SUB2API_SETUP_FILE=%~f0"',
    'powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$s=[IO.File]::ReadAllText($env:SUB2API_SETUP_FILE); & ([scriptblock]::Create($s.Substring($s.LastIndexOf(\'# SUB2API_POWERSHELL_BEGIN\'))))"',
    'set "SUB2API_SETUP_EXIT=%ERRORLEVEL%"',
    'echo.',
    'pause',
    'exit /b %SUB2API_SETUP_EXIT%',
    '# SUB2API_POWERSHELL_BEGIN',
    `$setupPayload = '${encoded}'`,
    setupPowerShell
  ].join('\n').replace(/\r?\n/g, '\r\n')
}
