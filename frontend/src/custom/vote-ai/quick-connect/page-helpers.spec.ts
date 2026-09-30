import { describe, it, expect, vi } from 'vitest'
import { findImageWorkbench, parseConnectKeyId, readAllConnectKeys } from './page-helpers'
import type { ApiKey, CustomMenuItem } from '@/types'

describe('get started key loading and navigation', () => {
  it('accepts only a positive safe integer ID, never a credential or array', () => {
    expect(parseConnectKeyId('183')).toBe(183)
    for (const value of ['0', '-1', '1.5', 'sk-secret', '9007199254740992', ['183'], null]) expect(parseConnectKeyId(value)).toBeNull()
  })
  it('reads every unfiltered page and deduplicates keys', async () => {
    const read = vi.fn().mockResolvedValueOnce({ pages: 2, items: [{ id: 1 }, { id: 2 }] })
      .mockResolvedValueOnce({ pages: 2, items: [{ id: 2 }, { id: 3 }] })
    const controller = new AbortController()
    expect((await readAllConnectKeys(read, controller.signal)).map(k => k.id)).toEqual([1, 2, 3])
    expect(read).toHaveBeenLastCalledWith(2, controller.signal)
  })
  it('rejects partial results when a later page fails', async () => {
    const read = vi.fn().mockResolvedValueOnce({ pages: 2, items: [{ id: 1 }] }).mockRejectedValueOnce(new Error('offline'))
    await expect(readAllConnectKeys(read, new AbortController().signal)).rejects.toThrow('offline')
  })
  it('discards a response arriving after cancellation', async () => {
    const controller = new AbortController()
    const read = vi.fn().mockImplementation(async () => {
      controller.abort()
      return { pages: 1, items: [{ id: 1 } as ApiKey] }
    })
    await expect(readAllConnectKeys(read, controller.signal)).rejects.toThrow()
  })
  it('uses only a user-visible trusted canvas entry without mutating menu order', () => {
    const item = (id: string, url: string, visibility: 'user' | 'admin', sort_order = 1) => ({ id, url, visibility, sort_order } as CustomMenuItem)
    const menus = [item('evil', 'https://canvas.vote520.com.attacker.example', 'user'), item('admin', 'https://canvas.vote520.com', 'admin'), item('image', 'https://canvas.vote520.com', 'user')]
    expect(findImageWorkbench(menus, 'https://ai.vote520.com')?.id).toBe('image')
    expect(findImageWorkbench(menus, 'https://unconfigured.example')).toBeUndefined()
    expect(menus.map(m => m.id)).toEqual(['evil', 'admin', 'image'])
  })
})
