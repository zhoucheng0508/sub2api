import { apiClient } from './client'
import { buildApiUrl } from './url'
import type { CodexOperatingSystem } from '@/utils/codexOneClick'

export type CCSwitchArchitecture = 'amd64' | 'arm64'

export interface CCSwitchDownload {
  download_url: string
  file_name: string
  release_url: string
  /** Same-origin endpoint that redirects to the validated binary asset. */
  direct_url?: string
  /** Git tag returned by GitHub (for example, v3.19.1). */
  version?: string
}

export interface CCSwitchDownloadOptions {
  version?: string
  signal?: AbortSignal
}

export interface CCSwitchReleaseVersion {
  /** Version without the conventional `v` prefix. */
  version: string
  /** Exact GitHub release tag; pass this back to resolveCCSwitchDownload. */
  tag_name: string
  name?: string
  published_at?: string
  release_url: string
  prerelease?: boolean
}

export interface CCSwitchVersionList {
  versions: CCSwitchReleaseVersion[]
  latest_version?: string
}

export interface CCSwitchVersionListOptions {
  limit?: number
  signal?: AbortSignal
}

export function startCCSwitchDownload(download: string | CCSwitchDownload): void {
  const downloadURL = typeof download === 'string'
    ? download
    : (download.direct_url || download.download_url)
  window.location.assign(downloadURL)
}

export function buildCCSwitchDirectDownloadURL(
  os: CodexOperatingSystem,
  arch: CCSwitchArchitecture,
  version?: string
): string {
  const params = new URLSearchParams({ os, arch })
  if (version?.trim()) params.set('version', version.trim())
  return `${buildApiUrl('/downloads/cc-switch/file')}?${params.toString()}`
}

// Alias with a shorter name for callers that treat the endpoint as the
// canonical download URL. Keep the explicit variant above for readability.
export const buildCCSwitchDownloadURL = buildCCSwitchDirectDownloadURL

export function listCCSwitchVersions(
  signal?: AbortSignal
): Promise<CCSwitchVersionList>
export function listCCSwitchVersions(
  limit?: number,
  signal?: AbortSignal
): Promise<CCSwitchVersionList>
export function listCCSwitchVersions(
  options?: CCSwitchVersionListOptions
): Promise<CCSwitchVersionList>
export async function listCCSwitchVersions(
  limitOrOptions?: number | AbortSignal | CCSwitchVersionListOptions,
  signal?: AbortSignal
): Promise<CCSwitchVersionList> {
  let limit: number | undefined
  let requestSignal = signal
  if (typeof limitOrOptions === 'number') {
    limit = limitOrOptions
  } else if (isAbortSignal(limitOrOptions)) {
    requestSignal = limitOrOptions
  } else if (limitOrOptions) {
    limit = limitOrOptions.limit
    requestSignal = limitOrOptions.signal || signal
  }
  const params = limit === undefined ? undefined : { limit }
  const response = await apiClient.get<CCSwitchVersionList>('/downloads/cc-switch/versions', {
    ...(params ? { params } : {}),
    signal: requestSignal,
  })
  return response.data
}

function isAbortSignal(value: unknown): value is AbortSignal {
  return Boolean(value && typeof value === 'object' && 'aborted' in value && 'addEventListener' in value)
}

function parseCCSwitchDownloadOptions(
  versionOrOptions?: string | AbortSignal | CCSwitchDownloadOptions,
  signal?: AbortSignal
): CCSwitchDownloadOptions {
  if (typeof versionOrOptions === 'string') {
    return { version: versionOrOptions, signal }
  }
  if (isAbortSignal(versionOrOptions)) {
    return { signal: versionOrOptions }
  }
  return { ...(versionOrOptions || {}), signal: versionOrOptions?.signal || signal }
}

export function resolveCCSwitchDownload(
  os: CodexOperatingSystem,
  arch: CCSwitchArchitecture,
  signal?: AbortSignal
): Promise<CCSwitchDownload>
export function resolveCCSwitchDownload(
  os: CodexOperatingSystem,
  arch: CCSwitchArchitecture,
  version?: string,
  signal?: AbortSignal
): Promise<CCSwitchDownload>
export function resolveCCSwitchDownload(
  os: CodexOperatingSystem,
  arch: CCSwitchArchitecture,
  options?: CCSwitchDownloadOptions
): Promise<CCSwitchDownload>
export async function resolveCCSwitchDownload(
  os: CodexOperatingSystem,
  arch: CCSwitchArchitecture,
  versionOrOptions?: string | AbortSignal | CCSwitchDownloadOptions,
  signal?: AbortSignal
): Promise<CCSwitchDownload> {
  const options = parseCCSwitchDownloadOptions(versionOrOptions, signal)
  const params: { os: CodexOperatingSystem; arch: CCSwitchArchitecture; version?: string } = { os, arch }
  if (options.version?.trim()) params.version = options.version.trim()
  const response = await apiClient.get<CCSwitchDownload>('/downloads/cc-switch', {
    params,
    signal: options.signal
  })
  return response.data
}
