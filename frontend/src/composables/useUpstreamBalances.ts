import { onScopeDispose, reactive, watch, type Ref } from 'vue'
import { getUpstreamBalance, type UpstreamBalance } from '@/api/admin/upstreamBalance'

export interface BalanceState {
  data?: UpstreamBalance
  loading: boolean
  failed: boolean
  fetchedAt: number
  identity: string
}

type BalanceAccount = { id: number; type: string; updated_at: string }
const TTL = 5 * 60_000

// Scoped to this page: no persisted balances or credentials in browser storage.
export function useUpstreamBalances(accounts: Ref<BalanceAccount[]>, enabled: Ref<boolean>) {
  const states = reactive(new Map<number, BalanceState>())
  const controller = new AbortController()
  let active = 0
  let disposed = false
  const queue: Array<() => Promise<void>> = []

  function drain() {
    while (!disposed && active < 3 && queue.length) {
      active++
      void queue.shift()!().finally(() => { active--; drain() })
    }
  }

  function refresh(account: BalanceAccount, force = false) {
    if (disposed || account.type !== 'apikey' || !enabled.value) return
    const previous = states.get(account.id)
    const identity = account.updated_at
    if (previous?.identity === identity && (previous.loading || (!force && Date.now() - previous.fetchedAt < TTL))) return
    states.set(account.id, {
      data: previous?.identity === identity ? previous.data : undefined,
      loading: true, failed: false, fetchedAt: previous?.fetchedAt ?? 0, identity
    })
    const state = states.get(account.id)!
    queue.push(async () => {
      if (!accounts.value.some(row => row.id === account.id && row.updated_at === identity) || !enabled.value) {
        if (states.get(account.id) === state) states.delete(account.id)
        return
      }
      try {
        const data = await getUpstreamBalance(account.id, controller.signal)
        if (disposed || states.get(account.id) !== state) return
        state.failed = data.status !== 'ok'
        if (data.status === 'ok' || !state.data) state.data = data
      } catch {
        if (!disposed) state.failed = true
      } finally {
        state.loading = false
        state.fetchedAt = Date.now()
      }
    })
    drain()
  }

  function refreshAll(force = true) {
    for (const account of accounts.value) refresh(account, force)
  }
  watch(() => [enabled.value, ...accounts.value.map(row => `${row.id}:${row.type}:${row.updated_at}`)], () => refreshAll(false), { immediate: true })
  const timer = setInterval(() => {
    if (!document.hidden) refreshAll(false)
  }, 60_000)
  onScopeDispose(() => {
    disposed = true
    queue.length = 0
    clearInterval(timer)
    controller.abort()
  })
  return { states, refresh, refreshAll }
}
