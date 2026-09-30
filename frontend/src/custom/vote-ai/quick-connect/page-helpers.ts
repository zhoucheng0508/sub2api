import type { ApiKey, CustomMenuItem, PaginatedResponse } from '@/types'
import { trustedCanvasUrl } from '@/custom/vote-ai/canvas-key-bridge'

export function parseConnectKeyId(value: unknown): number | null {
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value)) return null
  const id = Number(value)
  return Number.isSafeInteger(id) ? id : null
}

/** Use only the existing trusted iframe entry; never put credentials in a route. */
export function findImageWorkbench(items: CustomMenuItem[], origin: string): CustomMenuItem | undefined {
  return items.filter(item => item.visibility === 'user' && trustedCanvasUrl(origin, item.url))
    .sort((a, b) => a.sort_order - b.sort_order)[0]
}

/** Atomic result: a failed later page must not leave an apparently complete selector. */
export async function readAllConnectKeys(
  readPage: (page: number, signal: AbortSignal) => Promise<PaginatedResponse<ApiKey>>,
  signal: AbortSignal,
): Promise<ApiKey[]> {
  const keys = new Map<number, ApiKey>()
  let page = 1
  let pages = 1
  do {
    signal.throwIfAborted()
    const result = await readPage(page, signal)
    signal.throwIfAborted()
    if (!Number.isSafeInteger(result.pages) || result.pages < 0) throw new Error('Invalid key pagination')
    pages = result.pages
    for (const key of result.items) keys.set(key.id, key)
    page++
  } while (page <= pages)
  return [...keys.values()]
}
