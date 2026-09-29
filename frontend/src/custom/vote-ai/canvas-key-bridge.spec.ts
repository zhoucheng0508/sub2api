import { describe, expect, it, vi } from 'vitest'
import { createCanvasKeyBridge, supportsCanvasKind, unavailableReason, trustedCanvasUrl } from './canvas-key-bridge'
import type { ApiKey } from '@/types'
const origin = 'https://canvas.vote520.com'
const key = (id: number, image = true) => ({ id, user_id: 7, group_id: image ? 34 : 6, name: `Key ${id}`, key: `sk-private-fixture-${id}`, status: 'active', quota: 0, quota_used: 0, expires_at: null,
  group: { name: image ? 'Images' : 'Text', status: 'active', platform: 'openai', allow_image_generation: image, image_only: image } } as ApiKey)
function setup() {
  const target = { postMessage: vi.fn() }
  const frame = { src: origin, contentWindow: target } as unknown as HTMLIFrameElement
  let identity = 'session-a'
  const list = vi.fn(async (_page: number) => ({ items: [key(1), key(2, false)], pages: 1 }))
  const get = vi.fn(async (id: number) => key(id))
  const bridge = createCanvasKeyBridge({ origin: 'https://ai.vote520.com', frame: () => frame, identity: () => identity, userId: () => 7, list, get })
  const send = (data: object, overrides = {}) => bridge.receive({ origin, source: target, data: { protocol: 'vote-canvas-keys-v1', id: 'request', kind: 'image', ...data }, ...overrides } as MessageEvent)
  return { target, frame, list, get, bridge, send, changeIdentity: () => { identity = 'session-b' } }
}
describe('canvas credential bridge', () => {
  it('requires both exact origin and the configured iframe window', async () => {
    const s = setup(); await s.send({ action: 'list' }, { origin: 'https://evil.invalid' }); await s.send({ action: 'list' }, { source: {} })
    expect(s.list).not.toHaveBeenCalled(); expect(s.target.postMessage).not.toHaveBeenCalled()
  })
  it('returns metadata only, filters ownership and capabilities, and follows pagination', async () => {
    const s = setup(); s.list.mockImplementation(async page => ({ items: page === 1 ? [key(1), key(2, false), { ...key(3), user_id: 99 }] : [key(4)], pages: 2 }))
    await s.send({ action: 'list' }); const payload = s.target.postMessage.mock.calls[0]![0]
    expect(payload.keys.map((k: { id: number }) => k.id)).toEqual([1, 4]); expect(JSON.stringify(payload)).not.toContain('sk-private')
    expect(s.target.postMessage.mock.calls[0]![1]).toBe(origin)
  })
  it('does not provide a key that was not offered', async () => {
    const s = setup(); await s.send({ action: 'select', keyId: 1 }); expect(s.get).not.toHaveBeenCalled()
    expect(s.target.postMessage.mock.calls[0]![0].ok).toBe(false)
  })
  it('only returns the chosen key after a fresh ownership check', async () => {
    const s = setup(); await s.send({ action: 'list' }); await s.send({ action: 'select', keyId: 1 })
    expect(s.get).toHaveBeenCalledWith(1); expect(s.target.postMessage.mock.lastCall![0].groupId).toBe(34); expect(s.target.postMessage.mock.lastCall![0].key).toBe(key(1).key)
    s.get.mockResolvedValue({ ...key(1), user_id: 99 }); await s.send({ action: 'select', keyId: 1 }); expect(s.target.postMessage.mock.lastCall![0]).not.toHaveProperty('key')
  })
  it('refuses a key revoked after listing', async () => {
    const s = setup(); await s.send({ action: 'list' }); s.get.mockResolvedValue({ ...key(1), status: 'inactive' }); await s.send({ action: 'select', keyId: 1 })
    expect(s.target.postMessage.mock.lastCall![0].ok).toBe(false)
  })
  it('discards in-flight reads after an account switch', async () => {
    const s = setup(); s.list.mockImplementation(async () => { s.changeIdentity(); return { items: [key(1)], pages: 1 } }); await s.send({ action: 'list' }); expect(s.target.postMessage).not.toHaveBeenCalled()
  })
  it('does not send a secret after the iframe destination changes', async () => {
    const s = setup(); await s.send({ action: 'list' }); s.target.postMessage.mockClear()
    s.get.mockImplementation(async id => { s.frame.src = 'https://evil.invalid'; return key(id) }); await s.send({ action: 'select', keyId: 1 }); expect(s.target.postMessage).not.toHaveBeenCalled()
  })
  it('resets previously offered keys on logout', async () => {
    const s = setup(); await s.send({ action: 'list' }); s.bridge.reset(); await s.send({ action: 'select', keyId: 1 }); expect(s.get).not.toHaveBeenCalled()
  })
  it('separates image-only and text keys and reports unusable states', () => {
    expect(supportsCanvasKind(key(1), 'text')).toBe(false); expect(supportsCanvasKind(key(2, false), 'image')).toBe(false)
    expect(unavailableReason({ ...key(1), expires_at: '2000-01-01' })).toBe('已过期')
    expect(unavailableReason({ ...key(1), quota: 1, quota_used: 1 })).toBe('额度已用完')
  })
  it('allows only the configured production and test host pairs', () => {
    expect(trustedCanvasUrl('https://ai.vote520.com', origin)).toBe(true)
    expect(trustedCanvasUrl('https://evil.invalid', origin)).toBe(false)
    expect(trustedCanvasUrl('https://ai.vote520.com', `${origin}.evil.invalid`)).toBe(false)
  })
})
