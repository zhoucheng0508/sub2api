import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { useUpstreamBalances } from '../useUpstreamBalances'
import { getUpstreamBalance, type UpstreamBalance } from '@/api/admin/upstreamBalance'

vi.mock('@/api/admin/upstreamBalance', () => ({ getUpstreamBalance: vi.fn() }))
const query = vi.mocked(getUpstreamBalance)
const snapshot: UpstreamBalance = { status: 'ok', amount: 0, unit: 'USD', scope: 'wallet', provider: 'sub2api', checked_at: '2026-10-10T00:00:00Z' }
let scope: ReturnType<typeof effectScope>

beforeEach(() => { scope = effectScope(); query.mockReset() })
afterEach(() => { scope.stop(); vi.useRealTimers() })

function setup(rows = [{ id: 1, type: 'apikey', updated_at: 'a' }]) {
  const accounts = ref(rows)
  const enabled = ref(true)
  const balances = scope.run(() => useUpstreamBalances(accounts, enabled))!
  return { accounts, enabled, ...balances }
}

describe('upstream balances', () => {
  it('keeps a real zero and caches it across list refreshes', async () => {
    query.mockResolvedValue(snapshot)
    const { accounts, states, refreshAll } = setup()
    await flushPromises()
    expect(states.get(1)?.data?.amount).toBe(0)
    accounts.value = [{ id: 1, type: 'apikey', updated_at: 'a' }]
    await nextTick()
    refreshAll(false)
    expect(query).toHaveBeenCalledTimes(1)
  })

  it('preserves the last amount and marks it stale on refresh failure', async () => {
    query.mockResolvedValue(snapshot)
    const { states, refreshAll } = setup()
    await flushPromises()
    query.mockResolvedValue({ status: 'unavailable', checked_at: '2026-10-10T00:05:00Z' })
    refreshAll()
    await flushPromises()
    expect(states.get(1)?.data).toEqual(snapshot)
    expect(states.get(1)?.failed).toBe(true)
    query.mockRejectedValue(new Error('offline'))
    refreshAll()
    await flushPromises()
    expect(states.get(1)?.data).toEqual(snapshot)
    expect(states.get(1)?.loading).toBe(false)
  })

  it('does not query OAuth accounts or hidden balances', async () => {
    const { enabled, accounts } = setup([{ id: 1, type: 'oauth', updated_at: 'a' }])
    await flushPromises()
    expect(query).not.toHaveBeenCalled()
    enabled.value = false
    accounts.value = [{ id: 2, type: 'apikey', updated_at: 'a' }]
    await flushPromises()
    expect(query).not.toHaveBeenCalled()
  })

  it('limits simultaneous requests and cancels queued work when disposed', async () => {
    const resolve: Array<(data: UpstreamBalance) => void> = []
    query.mockImplementation(() => new Promise(done => resolve.push(done)))
    setup(Array.from({ length: 8 }, (_, i) => ({ id: i + 1, type: 'apikey', updated_at: 'a' })))
    expect(query).toHaveBeenCalledTimes(3)
    resolve[0]!(snapshot)
    await flushPromises()
    expect(query).toHaveBeenCalledTimes(4)
    scope.stop()
    expect(query.mock.calls[0]![1]?.aborted).toBe(true)
    for (const done of resolve) done(snapshot)
    await flushPromises()
    expect(query).toHaveBeenCalledTimes(4)
  })

  it('discards an old result when account configuration changes', async () => {
    let first!: (value: UpstreamBalance) => void
    query.mockImplementationOnce(() => new Promise(resolve => { first = resolve }))
    const { accounts, states } = setup()
    query.mockResolvedValue({ ...snapshot, amount: 8 })
    accounts.value = [{ id: 1, type: 'apikey', updated_at: 'b' }]
    await flushPromises()
    first(snapshot)
    await flushPromises()
    expect(states.get(1)?.data?.amount).toBe(8)
  })
})
