/**
 * Subscription Store
 * Global state management for user subscriptions with caching and deduplication
 */

import { defineStore } from 'pinia'
import { ref, computed, watch, onScopeDispose } from 'vue'
import subscriptionsAPI from '@/api/subscriptions'
import { useAuthStore } from '@/stores/auth'
import type { UserSubscription } from '@/types'

// Cache TTL: 60 seconds
const CACHE_TTL_MS = 60_000

export const useSubscriptionStore = defineStore('subscriptions', () => {
  const authStore = useAuthStore()
  // Each store instance and identity owns its own requests and cache.
  let requestGeneration = 0

  function identity(): string {
    return `${authStore.user?.id ?? ''}:${Boolean(authStore.token)}`
  }

  // State
  const activeSubscriptions = ref<UserSubscription[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const lastFetchedAt = ref<number | null>(null)

  // In-flight request deduplication
  let activePromise: Promise<UserSubscription[]> | null = null

  // Auto-refresh interval
  let pollerInterval: ReturnType<typeof setInterval> | null = null

  // Computed
  const hasActiveSubscriptions = computed(() => activeSubscriptions.value.length > 0)

  /**
   * Fetch active subscriptions with caching and deduplication
   * @param force - Force refresh even if cache is valid
   */
  async function fetchActiveSubscriptions(force = false): Promise<UserSubscription[]> {
    const now = Date.now()

    // A caller arriving during a forced refresh must share that request, not stale cache.
    if (activePromise && !force) {
      return activePromise
    }

    // Return cached data if valid
    if (
      !force &&
      loaded.value &&
      lastFetchedAt.value !== null &&
      now - lastFetchedAt.value < CACHE_TTL_MS
    ) {
      return activeSubscriptions.value
    }

    const currentGeneration = ++requestGeneration
    const requestIdentity = identity()
    const isCurrent = () => currentGeneration === requestGeneration && requestIdentity === identity()

    // Start new request
    loading.value = true
    const requestPromise = subscriptionsAPI
      .getActiveSubscriptions()
      .then((data) => {
        // Empty stale results are not cached and cannot leak a previous user's data
        // through the promise returned to a component.
        if (!isCurrent()) return []
        activeSubscriptions.value = data
        loaded.value = true
        lastFetchedAt.value = Date.now()
        return data
      })
      .catch((error) => {
        if (!isCurrent()) return []
        console.error('Failed to fetch active subscriptions:', error)
        throw error
      })
      .finally(() => {
        if (activePromise === requestPromise) {
          loading.value = false
          activePromise = null
        }
      })

    activePromise = requestPromise

    return activePromise
  }

  /**
   * Start auto-refresh polling 
   */
  function startPolling() {
    if (pollerInterval) return

    pollerInterval = setInterval(() => {
      fetchActiveSubscriptions(true).catch((error) => {
        console.error('Subscription polling failed:', error)
      })
    }, 5 * 60 * 1000)
  }

  /**
   * Stop auto-refresh polling
   */
  function stopPolling() {
    if (pollerInterval) {
      clearInterval(pollerInterval)
      pollerInterval = null
    }
  }

  /**
   * Clear all subscription data and stop polling
   */
  function clear() {
    requestGeneration++
    activePromise = null
    loading.value = false
    activeSubscriptions.value = []
    loaded.value = false
    lastFetchedAt.value = null
    stopPolling()
  }

  /**
   * Invalidate cache (force next fetch to reload)
   */
  function invalidateCache() {
    lastFetchedAt.value = null
  }

  watch(() => [authStore.user?.id, Boolean(authStore.token)] as const, (next, previous) => {
    if (next[0] !== previous[0] || next[1] !== previous[1]) clear()
  }, { flush: 'sync' })
  onScopeDispose(clear)

  return {
    // State
    activeSubscriptions,
    loading,
    hasActiveSubscriptions,

    // Actions
    fetchActiveSubscriptions,
    startPolling,
    stopPolling,
    clear,
    invalidateCache
  }
})
