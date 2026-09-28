import type { ApiKey } from '@/types'

export type CanvasKind = 'image' | 'text'
const protocol = 'vote-canvas-keys-v1'
const pairs: Record<string, string> = {
  'https://ai.vote520.com': 'https://canvas.vote520.com',
  'https://hermes.vote520.com:19443': 'https://hermes.vote520.com:19445',
}
export function trustedCanvasUrl(parentOrigin: string, url: string) {
  try { return Boolean(pairs[parentOrigin]) && new URL(url).origin === pairs[parentOrigin] } catch { return false }
}

export function supportsCanvasKind(key: ApiKey, kind: CanvasKind) {
  const group = key.group
  if (!group || group.platform !== 'openai') return false
  if (typeof group.image_only !== 'boolean') return false
  const imageOnly = group.image_only
  return kind === 'image' ? group.allow_image_generation && imageOnly : !imageOnly
}

export function unavailableReason(key: ApiKey) {
  if (key.status === 'expired' || (key.expires_at && Date.parse(key.expires_at) <= Date.now())) return '已过期'
  if (key.status === 'quota_exhausted' || (key.quota > 0 && key.quota_used >= key.quota)) return '额度已用完'
  if (key.status !== 'active') return '已停用'
  if (key.group?.status !== 'active') return '分组已停用'
  return ''
}

// The existing user API enforces ownership. Never use the administrator key API here.
export function createCanvasKeyBridge(deps: {
  origin: string
  frame: () => HTMLIFrameElement | null
  identity: () => string
  userId: () => number | undefined
  list: (page: number) => Promise<{ items: ApiKey[]; pages: number }>
  get: (id: number) => Promise<ApiKey>
}) {
  const canvasOrigin = pairs[deps.origin]
  let generation = 0
  const offered = new Set<string>()
  function frameTarget() {
    const frame = deps.frame()
    if (!canvasOrigin || !frame?.contentWindow) return null
    try { if (new URL(frame.src).origin !== canvasOrigin) return null } catch { return null }
    return frame.contentWindow
  }
  function reset() {
    generation++; offered.clear()
    frameTarget()?.postMessage({ protocol, action: 'reset', userId: deps.identity() ? deps.userId() : 0 }, canvasOrigin)
  }
  async function receive(event: MessageEvent) {
    const target = frameTarget(), request = event.data
    if (!target || event.source !== target || event.origin !== canvasOrigin || request?.protocol !== protocol) return
    if (typeof request.id !== 'string' || !['hello', 'list', 'select'].includes(request.action)) return
    const identity = deps.identity(), userId = deps.userId(), currentGeneration = generation
    if (!identity || !userId) { reset(); return }
    const valid = () => generation === currentGeneration && identity === deps.identity() && target === frameTarget()
    const reply = (payload: object) => { if (valid()) target.postMessage({ protocol, id: request.id, action: request.action, userId, ...payload }, canvasOrigin) }
    if (request.action === 'hello') { reply({ ok: true }); return }
    const kind = request.kind as CanvasKind
    if (kind !== 'image' && kind !== 'text') return
    try {
      if (request.action === 'list') {
        const keys: object[] = []
        let page = 1, pages = 1
        do {
          const result = await deps.list(page)
          if (!valid()) return
          pages = result.pages
          for (const key of result.items) {
            if (key.user_id !== userId || !supportsCanvasKind(key, kind)) continue
            const reason = unavailableReason(key)
            keys.push({ id: key.id, name: key.name, group: key.group!.name, suffix: key.key.slice(-4), reason })
            if (!reason) offered.add(`${identity}:${kind}:${key.id}`)
          }
          page++
        } while (page <= pages)
        reply({ ok: true, keys })
      } else {
        if (!Number.isSafeInteger(request.keyId) || !offered.has(`${identity}:${kind}:${request.keyId}`)) throw new Error('not_offered')
        const key = await deps.get(request.keyId)
        if (key.user_id !== userId || !supportsCanvasKind(key, kind) || unavailableReason(key)) throw new Error('unavailable')
        reply({ ok: true, key: key.key, keyId: key.id })
      }
    } catch { reply({ ok: false, error: '读取失败，请刷新密钥列表；确认登录、分组权限和密钥状态。' }) }
  }
  return { receive, reset }
}
