import { beforeEach, describe, expect, it, vi } from 'vitest'

const getMock = vi.hoisted(() => vi.fn())
const buildApiUrlMock = vi.hoisted(() => vi.fn())

vi.mock('@/api/client', () => ({
  apiClient: {
    get: getMock,
  },
}))

vi.mock('@/api/url', () => ({
  buildApiUrl: buildApiUrlMock,
}))

import {
  buildCCSwitchDirectDownloadURL,
  listCCSwitchVersions,
  resolveCCSwitchDownload,
} from '@/api/downloads'

describe('CC Switch download API', () => {
  beforeEach(() => {
    getMock.mockReset()
    buildApiUrlMock.mockReset()
    buildApiUrlMock.mockImplementation((path: string) => `/api/v1${path}`)
    getMock.mockResolvedValue({
      data: {
        download_url: 'https://github.com/farion1231/cc-switch/releases/download/v3.19.1/asset.dmg',
        file_name: 'asset.dmg',
        release_url: 'https://github.com/farion1231/cc-switch/releases/tag/v3.19.1',
        version: 'v3.19.1',
      },
    })
  })

  it('keeps the legacy AbortSignal argument and omits version by default', async () => {
    const signal = new AbortController().signal

    await resolveCCSwitchDownload('macos', 'arm64', signal)

    expect(getMock).toHaveBeenCalledWith('/downloads/cc-switch', {
      params: { os: 'macos', arch: 'arm64' },
      signal,
    })
  })

  it('sends an explicitly requested release version', async () => {
    const signal = new AbortController().signal

    await resolveCCSwitchDownload('windows', 'amd64', '3.19.1', signal)

    expect(getMock).toHaveBeenCalledWith('/downloads/cc-switch', {
      params: { os: 'windows', arch: 'amd64', version: '3.19.1' },
      signal,
    })
  })

  it('accepts an options object for callers that need named arguments', async () => {
    const signal = new AbortController().signal

    await resolveCCSwitchDownload('linux', 'arm64', { version: 'v3.19.1', signal })

    expect(getMock).toHaveBeenCalledWith('/downloads/cc-switch', {
      params: { os: 'linux', arch: 'arm64', version: 'v3.19.1' },
      signal,
    })
  })

  it('builds a same-origin binary URL with encoded version', () => {
    expect(buildCCSwitchDirectDownloadURL('macos', 'arm64', 'v3.19.1-beta+1')).toBe(
      '/api/v1/downloads/cc-switch/file?os=macos&arch=arm64&version=v3.19.1-beta%2B1',
    )
  })

  it('honors a custom API prefix when building the direct download URL', () => {
    buildApiUrlMock.mockReturnValue('https://downloads.example.test/custom/cc-switch/file')

    expect(buildCCSwitchDirectDownloadURL('windows', 'amd64', 'v3.20.1')).toBe(
      'https://downloads.example.test/custom/cc-switch/file?os=windows&arch=amd64&version=v3.20.1',
    )
  })

  it('loads a bounded release list for the version picker', async () => {
    const signal = new AbortController().signal

    await listCCSwitchVersions(12, signal)

    expect(getMock).toHaveBeenCalledWith('/downloads/cc-switch/versions', {
      params: { limit: 12 },
      signal,
    })
  })
})
