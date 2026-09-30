import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import type { UserSubscription } from '@/types'
import { useSubscriptionStore } from '../subscriptions'

const mocks = vi.hoisted(() => ({ getActive: vi.fn(), auth: null as any }))
vi.mock('@/api/subscriptions', () => ({ default: { getActiveSubscriptions: mocks.getActive } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))

function subscription(id: number, userId: number) {
  return { id, user_id: userId } as UserSubscription
}

function deferred() {
  let resolve!: (value: UserSubscription[]) => void
  let reject!: (error: Error) => void
  const promise = new Promise<UserSubscription[]>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.useFakeTimers()
  mocks.auth = reactive({ user: { id: 1 }, token: 'first-token' })
  mocks.getActive.mockReset()
  mocks.getActive.mockResolvedValue([])
  vi.spyOn(console, 'error').mockImplementation(() => {})
})
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

describe('subscription identity isolation', () => {
  it('clears cached subscriptions synchronously on A-to-B identity switch', async () => {
    mocks.getActive.mockResolvedValueOnce([subscription(10, 1)])
      .mockResolvedValueOnce([subscription(20, 2)])
    const store = useSubscriptionStore()
    await store.fetchActiveSubscriptions()
    vi.advanceTimersByTime(10_000)
    mocks.auth.user = { id: 2 }
    expect(store.activeSubscriptions).toEqual([])
    expect(store.loading).toBe(false)
    expect(await store.fetchActiveSubscriptions()).toEqual([subscription(20, 2)])
    expect(mocks.getActive).toHaveBeenCalledTimes(2)
  })

  it('does not reuse a 60-second cache after A-to-B-to-A switching', async () => {
    mocks.getActive.mockResolvedValueOnce([subscription(10, 1)])
      .mockResolvedValueOnce([subscription(11, 1)])
    const store = useSubscriptionStore()
    await store.fetchActiveSubscriptions()
    mocks.auth.user = { id: 2 }
    mocks.auth.user = { id: 1 }
    expect(await store.fetchActiveSubscriptions()).toEqual([subscription(11, 1)])
    expect(mocks.getActive).toHaveBeenCalledTimes(2)
  })

  it('never returns old account data or changes new loading state when a request finishes late', async () => {
    const old = deferred()
    const current = deferred()
    mocks.getActive.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const store = useSubscriptionStore()
    const first = store.fetchActiveSubscriptions()
    mocks.auth.user = { id: 2 }
    const second = store.fetchActiveSubscriptions()
    old.resolve([subscription(10, 1)])
    expect(await first).toEqual([])
    expect(store.activeSubscriptions).toEqual([])
    expect(store.loading).toBe(true)
    current.resolve([subscription(20, 2)])
    expect(await second).toEqual([subscription(20, 2)])
    expect(store.activeSubscriptions).toEqual([subscription(20, 2)])
    expect(store.loading).toBe(false)
  })

  it.each(['resolve', 'reject'] as const)('invalidates a late request on logout (%s)', async (outcome) => {
    const pending = deferred()
    mocks.getActive.mockReturnValueOnce(pending.promise)
    const store = useSubscriptionStore()
    const request = store.fetchActiveSubscriptions()
    mocks.auth.token = null
    expect(store.loading).toBe(false)
    expect(store.activeSubscriptions).toEqual([])
    if (outcome === 'resolve') pending.resolve([subscription(10, 1)])
    else pending.reject(new Error('old account error'))
    expect(await request).toEqual([])
    expect(store.activeSubscriptions).toEqual([])
    expect(console.error).not.toHaveBeenCalled()
  })

  it('stops the old account poller when identity changes', () => {
    const store = useSubscriptionStore()
    store.startPolling()
    mocks.auth.user = { id: 2 }
    vi.advanceTimersByTime(10 * 60_000)
    expect(mocks.getActive).not.toHaveBeenCalled()
    store.startPolling()
    vi.advanceTimersByTime(5 * 60_000)
    expect(mocks.getActive).toHaveBeenCalledTimes(1)
    store.stopPolling()
  })

  it('preserves cache, pending result and poller when only a same-user token rotates', async () => {
    const pending = deferred()
    mocks.getActive.mockReturnValueOnce(pending.promise).mockResolvedValue([subscription(11, 1)])
    const store = useSubscriptionStore()
    store.startPolling()
    const request = store.fetchActiveSubscriptions()
    mocks.auth.token = 'rotated-token'
    expect(store.loading).toBe(true)
    pending.resolve([subscription(10, 1)])
    expect(await request).toEqual([subscription(10, 1)])
    mocks.auth.token = 'another-rotated-token'
    expect(await store.fetchActiveSubscriptions()).toEqual([subscription(10, 1)])
    expect(mocks.getActive).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(5 * 60_000)
    expect(mocks.getActive).toHaveBeenCalledTimes(2)
    store.stopPolling()
  })

  it('allows only the latest forced request to return and cache data', async () => {
    const older = deferred()
    const newer = deferred()
    mocks.getActive.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const store = useSubscriptionStore()
    const first = store.fetchActiveSubscriptions(true)
    const second = store.fetchActiveSubscriptions(true)
    const shared = store.fetchActiveSubscriptions()
    newer.resolve([subscription(20, 1)])
    expect(await second).toEqual([subscription(20, 1)])
    expect(await shared).toEqual([subscription(20, 1)])
    older.resolve([subscription(10, 1)])
    expect(await first).toEqual([])
    expect(store.activeSubscriptions).toEqual([subscription(20, 1)])
    expect(await store.fetchActiveSubscriptions()).toEqual([subscription(20, 1)])
    expect(mocks.getActive).toHaveBeenCalledTimes(2)
  })

  it('shares a forced refresh instead of an earlier still-valid cache', async () => {
    const pending = deferred()
    mocks.getActive.mockResolvedValueOnce([subscription(10, 1)]).mockReturnValueOnce(pending.promise)
    const store = useSubscriptionStore()
    await store.fetchActiveSubscriptions()
    const refresh = store.fetchActiveSubscriptions(true)
    const ordinary = store.fetchActiveSubscriptions()
    pending.resolve([subscription(11, 1)])
    expect(await refresh).toEqual([subscription(11, 1)])
    expect(await ordinary).toEqual([subscription(11, 1)])
    expect(mocks.getActive).toHaveBeenCalledTimes(2)
  })

  it('isolates request generations between independent Pinia stores', async () => {
    const firstPending = deferred()
    const secondPending = deferred()
    mocks.getActive.mockReturnValueOnce(firstPending.promise).mockReturnValueOnce(secondPending.promise)
    const firstStore = useSubscriptionStore(createPinia())
    const secondStore = useSubscriptionStore(createPinia())
    const first = firstStore.fetchActiveSubscriptions()
    const second = secondStore.fetchActiveSubscriptions()
    firstPending.resolve([subscription(10, 1)])
    secondPending.resolve([subscription(20, 1)])
    expect(await first).toEqual([subscription(10, 1)])
    expect(await second).toEqual([subscription(20, 1)])
  })
})
