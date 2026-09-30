import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { authAPI } from '@/api/auth'
import subscriptionsAPI from '@/api/subscriptions'
import { paymentAPI } from '@/api/payment'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { hasUsableSubscription, resolveFundingPhase } from './funding-readiness'

/** Read-only, component-local funding state. Late requests never update another account. */
export function useFundingReadiness() {
  const authStore = useAuthStore()
  const appStore = useAppStore()
  const balance = ref<number | null>(null)
  const loading = ref(false)
  const hasActiveSubscription = ref<boolean | null>(null)
  const settingsReady = ref(false)
  const onlineAvailable = ref<boolean | null>(null)
  const runMode = ref<'standard' | 'simple' | null>(null)
  const checkoutBalanceDisabled = ref<boolean | null>(null)
  const purchasableSubscription = ref(false)
  let generation = 0
  let mounted = false
  let disposed = false
  let inFlight: { session: string; promise: Promise<void> } | null = null

  const phase = computed(() => resolveFundingPhase({
    runMode: runMode.value,
    balance: balance.value,
    hasActiveSubscription: hasActiveSubscription.value
  }))
  const canRedeem = computed(() => runMode.value === 'standard')
  const subscriptionOnly = computed(() => runMode.value === 'standard' && onlineAvailable.value === true
    && checkoutBalanceDisabled.value === true && purchasableSubscription.value)

  function sessionKey(): string {
    return `${authStore.user?.id ?? ''}:${Boolean(authStore.token)}`
  }

  function reset() {
    balance.value = null
    hasActiveSubscription.value = null
    runMode.value = null
    loading.value = false
    onlineAvailable.value = null
    checkoutBalanceDisabled.value = null
    purchasableSubscription.value = false
    settingsReady.value = false
  }

  async function refresh(): Promise<void> {
    const session = sessionKey()
    if (disposed || !authStore.token || !authStore.user?.id) return
    if (inFlight?.session === session) return inFlight.promise
    const userId = authStore.user.id
    const requestGeneration = ++generation
    const current = () => !disposed && generation === requestGeneration && sessionKey() === session
    loading.value = true

    const request = (async () => {
      // Clearing stale facts makes a failed refresh honest, while token rotation alone keeps them.
      balance.value = null
      hasActiveSubscription.value = null
      onlineAvailable.value = null
      checkoutBalanceDisabled.value = null
      purchasableSubscription.value = false
      const publicSettingsRequest = appStore.fetchPublicSettings().catch(() => null)
      try {
        const profile = await authAPI.getCurrentUser()
        if (!current()) return
        if (profile.data.id !== userId) {
          runMode.value = null
          return
        }
        runMode.value = profile.data.run_mode ?? authStore.runMode
        // Backend balance is already available balance; frozen_balance must not be deducted again.
        balance.value = Number.isFinite(profile.data.balance) ? profile.data.balance : null
      } catch {
        if (current()) runMode.value = null
      }

      await publicSettingsRequest
      if (!current()) return
      const confirmedSettings = appStore.publicSettingsLoaded ? appStore.cachedPublicSettings : null
      settingsReady.value = confirmedSettings !== null
      if (runMode.value === 'simple') {
        onlineAvailable.value = false
        return
      }
      if (runMode.value !== 'standard' || !confirmedSettings) return

      const subscriptionsRequest = confirmedSettings.subscription_enabled === false
        ? Promise.resolve(false)
        : subscriptionsAPI.getActiveSubscriptions()
          .then((subscriptions) => subscriptions.some((subscription) => hasUsableSubscription(subscription)))
          .catch(() => null)
      const checkoutRequest = confirmedSettings.payment_enabled === false
        ? Promise.resolve(null)
        : paymentAPI.getCheckoutInfo().then((response) => response.data).catch(() => null)
      const [subscriptionAvailable, checkout] = await Promise.all([subscriptionsRequest, checkoutRequest])
      if (!current()) return
      hasActiveSubscription.value = subscriptionAvailable
      if (confirmedSettings.payment_enabled === false) {
        onlineAvailable.value = false
      } else if (checkout) {
        checkoutBalanceDisabled.value = checkout.balance_disabled
        purchasableSubscription.value = confirmedSettings.subscription_enabled !== false
          && checkout.plans.some((plan) => plan.for_sale === true)
        const availableMethod = Object.values(checkout.methods).some((method) => method.available === true)
        onlineAvailable.value = availableMethod
          && (checkout.balance_disabled === false || purchasableSubscription.value)
      }
    })().finally(() => {
      if (current()) loading.value = false
      if (inFlight?.promise === request) inFlight = null
    })
    inFlight = { session, promise: request }
    return request
  }

  watch(() => [authStore.user?.id, Boolean(authStore.token)] as const, (next, previous) => {
    if (next[0] === previous[0] && next[1] === previous[1]) return
    generation++
    inFlight = null
    reset()
    if (mounted) void refresh()
  }, { flush: 'sync' })
  onMounted(() => {
    mounted = true
    void refresh()
  })
  onUnmounted(() => {
    disposed = true
    generation++
  })

  return { phase, balance, loading, hasActiveSubscription, settingsReady, onlineAvailable,
    canRedeem, subscriptionOnly, refresh }
}
