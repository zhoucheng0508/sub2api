import { apiClient } from './client'
import type { CodexOperatingSystem } from '@/utils/codexOneClick'

export type CCSwitchArchitecture = 'amd64' | 'arm64'

export interface CCSwitchDownload {
  download_url: string
  file_name: string
  release_url: string
}

export function startCCSwitchDownload(downloadURL: string): void {
  window.location.assign(downloadURL)
}

export async function resolveCCSwitchDownload(
  os: CodexOperatingSystem,
  arch: CCSwitchArchitecture,
  signal?: AbortSignal
): Promise<CCSwitchDownload> {
  const response = await apiClient.get<CCSwitchDownload>('/downloads/cc-switch', {
    params: { os, arch },
    signal
  })
  return response.data
}
