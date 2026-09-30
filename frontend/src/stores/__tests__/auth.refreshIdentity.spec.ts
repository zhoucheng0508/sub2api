import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../auth'

const { login, getCurrentUser } = vi.hoisted(() => ({ login: vi.fn(), getCurrentUser: vi.fn() }))
vi.mock('@/api', () => ({ authAPI: { login, getCurrentUser, logout: vi.fn().mockResolvedValue(undefined) }, isTotp2FARequired: () => false, passkeyAPI: {} }))
function user(id: number, balance = 10) {
  return { id, username: `user${id}`, email: `user${id}@example.invalid`, role: 'user', balance, concurrency: 1, status: 'active', allowed_groups: [], created_at: '2026-09-30', updated_at: '2026-09-30' }
}
async function signIn(id: number) {
  login.mockResolvedValueOnce({ access_token: `session-${id}`, refresh_token: `refresh-${id}`, expires_in: 3600, token_type: 'Bearer', user: user(id) })
  const store = useAuthStore(); await store.login({ email: `user${id}@example.invalid`, password: 'fixture-only' }); return store
}
beforeEach(() => { vi.useFakeTimers(); localStorage.clear(); vi.clearAllMocks(); setActivePinia(createPinia()) })
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers(); localStorage.clear() })

describe('profile refresh session ownership', () => {
  it('cannot overwrite another account with a late profile', async () => {
    const store = await signIn(1)
    let finish!: (value: unknown) => void
    getCurrentUser.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const pending = store.refreshUser(); const rejected = expect(pending).rejects.toThrow('Account session changed')
    await signIn(2); finish({ data: user(1, 999) }); await rejected
    expect(store.user?.id).toBe(2)
    expect(JSON.parse(localStorage.getItem('auth_user')!).id).toBe(2)
  })
  it('does not resurrect a logged-out account', async () => {
    const store = await signIn(1)
    let finish!: (value: unknown) => void
    getCurrentUser.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const pending = store.refreshUser(); const rejected = expect(pending).rejects.toThrow('Account session changed')
    await store.logout(); finish({ data: user(1, 999) }); await rejected
    expect(store.user).toBeNull(); expect(localStorage.getItem('auth_user')).toBeNull()
  })
  it('cannot let an old 401 clear a newer login', async () => {
    const store = await signIn(1)
    let fail!: (error: unknown) => void
    getCurrentUser.mockImplementationOnce(() => new Promise((_resolve, reject) => { fail = reject }))
    const pending = store.refreshUser(); const rejected = expect(pending).rejects.toEqual({ status: 401 })
    await signIn(2); fail({ status: 401 }); await rejected
    expect(store.user?.id).toBe(2); expect(store.token).toBe('session-2')
  })
  it('keeps a valid profile read through a same-user token rotation', async () => {
    const store = await signIn(1)
    let finish!: (value: unknown) => void
    getCurrentUser.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const pending = store.refreshUser(); store.token = 'rotated-session'; finish({ data: user(1, 20) })
    await expect(pending).resolves.toMatchObject({ id: 1, balance: 20 })
    expect(store.token).toBe('rotated-session'); expect(store.user?.balance).toBe(20)
  })
  it('keeps the latest profile when concurrent reads complete out of order', async () => {
    const store = await signIn(1)
    let finishOld!: (value: unknown) => void
    getCurrentUser.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
    const old = store.refreshUser(); const rejected = expect(old).rejects.toThrow('Account session changed')
    getCurrentUser.mockResolvedValueOnce({ data: user(1, 30) }); await store.refreshUser()
    finishOld({ data: user(1, 5) }); await rejected
    expect(store.user?.balance).toBe(30)
  })
  it('does not let an old SSO completion clear a newer login', async () => {
    const store = useAuthStore()
    let finish!: (value: unknown) => void
    getCurrentUser.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const old = store.setToken('sso-session-1'); const rejected = expect(old).rejects.toThrow('Account session changed')
    await signIn(2); finish({ data: user(1, 999) }); await rejected
    expect(store.user?.id).toBe(2); expect(store.token).toBe('session-2')
    expect(localStorage.getItem('auth_token')).toBe('session-2')
  })
  it('keeps a newer SSO account when an earlier SSO response arrives', async () => {
    const store = useAuthStore()
    let finish!: (value: unknown) => void
    getCurrentUser.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const old = store.setToken('sso-session-1'); const rejected = expect(old).rejects.toThrow('Account session changed')
    getCurrentUser.mockResolvedValueOnce({ data: user(2, 20) }); await store.setToken('sso-session-2')
    finish({ data: user(1, 999) }); await rejected
    expect(store.user?.id).toBe(2); expect(store.token).toBe('sso-session-2')
    expect(localStorage.getItem('auth_token')).toBe('sso-session-2')
  })
})
