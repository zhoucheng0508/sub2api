import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'

enableAutoUnmount(afterEach)

const routeState = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
}))

const routerPush = vi.hoisted(() => vi.fn())
const pollOrderStatus = vi.hoisted(() => vi.fn())
const verifyOrder = vi.hoisted(() => vi.fn())
const verifyOrderPublic = vi.hoisted(() => vi.fn())
const resolveOrderPublicByResumeToken = vi.hoisted(() => vi.fn())
const refreshUser = vi.hoisted(() => vi.fn())
const fetchActiveSubscriptions = vi.hoisted(() => vi.fn())
const authState = vi.hoisted(() => ({
  isAuthenticated: true,
  token: 'session-a' as string | null,
  user: { id: 9, balance: 88 } as { id: number; balance: number } | null,
  setState: (_updates: { token?: string | null; isAuthenticated?: boolean; user?: { id: number; balance: number } | null }) => {},
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({ push: routerPush }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    pollOrderStatus,
  }),
}))

vi.mock('@/stores/auth', async () => {
  const { reactive } = await import('vue')
  const state = reactive(authState)
  authState.setState = (updates) => { Object.assign(state, updates) }
  return { useAuthStore: () => ({
    refreshUser,
    get isAuthenticated() { return state.isAuthenticated },
    get token() { return state.token },
    get user() { return state.user },
  }) }
})

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({ fetchActiveSubscriptions }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    verifyOrder,
    verifyOrderPublic,
    resolveOrderPublicByResumeToken,
  },
}))

import PaymentResultView from '../PaymentResultView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
import { formatPaymentAmount } from '@/components/payment/currency'

const orderFactory = (status: string) => ({
  id: 42,
  user_id: 9,
  amount: 88,
  pay_amount: 88,
  fee_rate: 0,
  payment_type: 'alipay',
  out_trade_no: 'sub2_20260420abcd1234',
  status,
  order_type: 'balance',
  created_at: '2026-04-20T12:00:00Z',
  expires_at: '2026-04-20T12:30:00Z',
  refund_amount: 0,
})

const recoverySnapshotFactory = (resumeToken: string) => ({
  orderId: 42,
  amount: 88,
  qrCode: '',
  expiresAt: '2099-01-01T00:10:00.000Z',
  paymentType: 'alipay',
  payUrl: 'https://pay.example.com/session/42',
  outTradeNo: 'sub2_20260420abcd1234',
  clientSecret: '',
  intentId: '',
  currency: '',
  countryCode: '',
  paymentEnv: '',
  payAmount: 88,
  orderType: 'balance',
  paymentMode: 'popup',
  resumeToken,
  createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
})

const usableSubscription = () => ({
  id: 71, user_id: 9, group_id: 3, status: 'active',
  starts_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z',
  group: { id: 3, status: 'active' },
})

describe('PaymentResultView', () => {
  beforeEach(() => {
    routeState.query = {}
    routerPush.mockReset()
    pollOrderStatus.mockReset()
    verifyOrder.mockReset()
    verifyOrderPublic.mockReset()
    resolveOrderPublicByResumeToken.mockReset()
    refreshUser.mockReset()
    refreshUser.mockResolvedValue({ id: 9, balance: 88 })
    fetchActiveSubscriptions.mockReset().mockResolvedValue([usableSubscription()])
    authState.setState({ isAuthenticated: true, token: 'session-a', user: { id: 9, balance: 88 } })
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders a pending state instead of a failure state when the restored order is still pending', async () => {
    routeState.query = {
      resume_token: 'resume-42',
      order_id: '999',
      status: 'success',
    }
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 42,
      amount: 88,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/session/42',
      outTradeNo: 'sub2_20260420abcd1234',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 88,
      orderType: 'balance',
      paymentMode: 'redirect',
      resumeToken: 'resume-42',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }))
    resolveOrderPublicByResumeToken.mockResolvedValue({
      data: orderFactory('PENDING'),
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-42')
    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.processing')
    expect(wrapper.text()).not.toContain('payment.result.success')
    expect(wrapper.text()).not.toContain('payment.result.failed')
  })

  it('prefers the public resume-token result over a stale restored DB snapshot', async () => {
    routeState.query = {
      resume_token: 'resume-authoritative',
      order_id: '42',
      status: 'success',
    }
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 42,
      amount: 88,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/session/42',
      outTradeNo: 'sub2_20260420abcd1234',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 88,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: 'resume-authoritative',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }))
    resolveOrderPublicByResumeToken.mockResolvedValue({
      data: {
        ...orderFactory('COMPLETED'),
        amount: 100,
        pay_amount: 103,
        fee_rate: 3,
      },
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-authoritative')
    expect(refreshUser).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.text()).toContain('103.00')
    expect(wrapper.text()).toContain('100.00')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('waits for completed fulfillment before refreshing the user balance', async () => {
    vi.useFakeTimers()
    routeState.query = {
      resume_token: 'resume-77',
    }
    window.localStorage.setItem(
      PAYMENT_RECOVERY_STORAGE_KEY,
      JSON.stringify(recoverySnapshotFactory('resume-77')),
    )
    resolveOrderPublicByResumeToken
      .mockResolvedValueOnce({
        data: orderFactory('PENDING'),
      })
      .mockResolvedValueOnce({
        data: orderFactory('COMPLETED'),
      })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledTimes(1)
    expect(refreshUser).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.processing')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).not.toBeNull()

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledTimes(2)
    expect(refreshUser).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.text()).not.toContain('payment.result.failed')
    expect(wrapper.text()).toContain('firstUseJourney.balanceCreditedBody')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('keeps the successful result when refreshing the user balance fails', async () => {
    routeState.query = {
      resume_token: 'resume-refresh-failure',
    }
    resolveOrderPublicByResumeToken.mockResolvedValue({
      data: orderFactory('COMPLETED'),
    })
    refreshUser.mockRejectedValueOnce(new Error('profile refresh failed'))

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(refreshUser).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.text()).not.toContain('payment.result.failed')
    expect(wrapper.text()).toContain('firstUseJourney.accountRefreshPendingBody')
    expect(wrapper.text()).not.toContain('firstUseJourney.balanceCreditedBody')
  })

  it('falls back to order_id polling when resume-token recovery fails', async () => {
    routeState.query = {
      resume_token: 'resume-fail',
      order_id: '77',
    }
    window.localStorage.setItem(
      PAYMENT_RECOVERY_STORAGE_KEY,
      JSON.stringify({
        ...recoverySnapshotFactory('resume-fail'),
        orderId: 42,
      }),
    )
    resolveOrderPublicByResumeToken.mockRejectedValueOnce(new Error('resume failed'))
    pollOrderStatus.mockResolvedValueOnce({
      ...orderFactory('PAID'),
      id: 77,
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-fail')
    expect(pollOrderStatus).toHaveBeenCalledWith(77)
    expect(verifyOrderPublic).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.processing')
    expect(wrapper.text()).toContain('firstUseJourney.settlementPendingBody')
    expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(false)
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).not.toBeNull()
  })

  it('falls back to public out_trade_no verification when resume_token recovery fails in legacy return flows', async () => {
    routeState.query = {
      resume_token: 'resume-fail',
      out_trade_no: 'legacy-should-not-run',
      trade_status: 'TRADE_SUCCESS',
    }
    resolveOrderPublicByResumeToken.mockRejectedValueOnce(new Error('resume failed'))
    verifyOrderPublic.mockResolvedValueOnce({
      data: {
        ...orderFactory('PAID'),
        out_trade_no: 'legacy-should-not-run',
      },
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-fail')
    expect(verifyOrderPublic).toHaveBeenCalledWith('legacy-should-not-run')
    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.processing')
    expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(false)
  })

  it('ignores a stale global recovery snapshot when legacy return markers do not identify the order', async () => {
    routeState.query = {
      trade_status: 'TRADE_SUCCESS',
    }
    window.localStorage.setItem(
      PAYMENT_RECOVERY_STORAGE_KEY,
      JSON.stringify(recoverySnapshotFactory('resume-stale')),
    )

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).not.toHaveBeenCalled()
    expect(verifyOrderPublic).not.toHaveBeenCalled()
    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.failed')
    expect(wrapper.text()).not.toContain('sub2_20260420abcd1234')
  })

  it('uses public out_trade_no verification when no signed resume context is available', async () => {
    routeState.query = {
      out_trade_no: 'legacy-123',
      trade_status: 'TRADE_SUCCESS',
    }
    verifyOrder.mockRejectedValue(new Error('auth required'))
    verifyOrderPublic.mockResolvedValue({
      data: orderFactory('PAID'),
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(verifyOrder).toHaveBeenCalledWith('legacy-123')
    expect(verifyOrderPublic).toHaveBeenCalledWith('legacy-123')
    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.processing')
    expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(false)
  })

  it('renders the minimal public out_trade_no verification result without payment_type', async () => {
    routeState.query = {
      out_trade_no: 'legacy-minimal',
      trade_status: 'TRADE_SUCCESS',
    }
    verifyOrder.mockRejectedValue(new Error('auth required'))
    verifyOrderPublic.mockResolvedValue({
      data: {
        out_trade_no: 'legacy-minimal',
        status: 'PAID',
        paid: true,
        created_at: '2026-04-20T12:00:00Z',
        expires_at: '2026-04-20T12:30:00Z',
      },
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('payment.result.processing')
    expect(wrapper.text()).toContain('legacy-minimal')
    expect(wrapper.text()).not.toContain('payment.orders.paymentMethod')
    expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(false)
  })

  it('prefers authenticated order verification before falling back to public lookup', async () => {
    routeState.query = {
      out_trade_no: 'auth-verify-123',
      trade_status: 'TRADE_SUCCESS',
    }
    verifyOrder.mockResolvedValue({
      data: orderFactory('COMPLETED'),
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(verifyOrder).toHaveBeenCalledWith('auth-verify-123')
    expect(verifyOrderPublic).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.success')
  })

  it('does not use public out_trade_no verification for bare order numbers without legacy return markers', async () => {
    routeState.query = {
      out_trade_no: 'legacy-bare',
    }

    mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(verifyOrderPublic).not.toHaveBeenCalled()
  })

  it('resolves order by resume token when local recovery snapshot is missing', async () => {
    routeState.query = {
      resume_token: 'resume-77',
    }
    resolveOrderPublicByResumeToken.mockResolvedValue({
      data: orderFactory('PAID'),
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-77')
    expect(wrapper.text()).toContain('payment.result.processing')
    expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(false)
  })

  it('uses the currency returned by the order API when rendering amounts', async () => {
    routeState.query = {
      resume_token: 'resume-hkd',
    }
    resolveOrderPublicByResumeToken.mockResolvedValue({
      data: {
        ...orderFactory('PAID'),
        currency: 'HKD',
        amount: 100,
        pay_amount: 103,
        fee_rate: 3,
      },
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain(formatPaymentAmount(103, 'HKD'))
  })

  it('normalizes aliased payment methods before rendering the label', async () => {
    routeState.query = {
      resume_token: 'resume-88',
    }
    resolveOrderPublicByResumeToken.mockResolvedValueOnce({
      data: {
        ...orderFactory('PAID'),
        payment_type: 'alipay_direct',
      },
    })

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('payment.methods.alipay')
    expect(wrapper.text()).not.toContain('payment.methods.alipay_direct')
  })

  it.each(['PENDING', 'PAID', 'RECHARGING', 'FAILED', 'EXPIRED', 'CANCELLED', 'UNKNOWN'])(
    'does not offer completed setup for %s, even when query parameters claim success', async (status) => {
      routeState.query = { resume_token: 'resume-unsettled', trade_status: 'TRADE_SUCCESS', status: 'COMPLETED' }
      resolveOrderPublicByResumeToken.mockResolvedValue({ data: orderFactory(status) })
      const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
      await flushPromises()

      expect(wrapper.find('[data-test="funding-completed"]').exists()).toBe(false)
      expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(false)
      expect(refreshUser).not.toHaveBeenCalled()
      expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
      expect(routerPush).not.toHaveBeenCalled()
      expect(wrapper.text()).toContain('payment.result.viewOrders')
    },
  )

  it('keeps paid fulfillment pending and polls until COMPLETED before refreshing or continuing', async () => {
    vi.useFakeTimers()
    routeState.query = { resume_token: 'resume-settling', scene: 'image' }
    resolveOrderPublicByResumeToken
      .mockResolvedValueOnce({ data: orderFactory('PAID') })
      .mockResolvedValueOnce({ data: orderFactory('RECHARGING') })
      .mockResolvedValueOnce({ data: orderFactory('COMPLETED') })
    const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('firstUseJourney.settlementPendingBody')
    expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(false)
    expect(refreshUser).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(false)
    expect(refreshUser).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(wrapper.text()).toContain('firstUseJourney.balanceCreditedBody')
    expect(refreshUser).toHaveBeenCalledOnce()
    expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
    await wrapper.get('[data-test="continue-setup"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith({ path: '/get-started', query: { scene: 'image' } })
  })

  it('refreshes a completed subscription only for the matching signed-in owner', async () => {
    routeState.query = { resume_token: 'resume-subscription', scene: 'code' }
    resolveOrderPublicByResumeToken.mockResolvedValue({ data: { ...orderFactory('COMPLETED'), order_type: 'subscription' } })
    const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
    await flushPromises()

    expect(refreshUser).toHaveBeenCalledOnce()
    expect(fetchActiveSubscriptions).toHaveBeenCalledWith(true)
    expect(wrapper.text()).toContain('firstUseJourney.subscriptionCreditedBody')
    expect(wrapper.text()).not.toContain('firstUseJourney.balanceCreditedBody')
    await wrapper.get('[data-test="continue-setup"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith({ path: '/get-started', query: { scene: 'code' } })
  })

  it('keeps COMPLETED success while showing that subscription information could not be refreshed', async () => {
    routeState.query = { resume_token: 'resume-subscription-failure' }
    resolveOrderPublicByResumeToken.mockResolvedValue({ data: { ...orderFactory('COMPLETED'), order_type: 'subscription' } })
    fetchActiveSubscriptions.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
    await flushPromises()

    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.text()).toContain('firstUseJourney.accountRefreshPendingBody')
    expect(wrapper.text()).not.toContain('firstUseJourney.subscriptionCreditedBody')
    expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(true)
  })

  it.each(['different-owner', 'unknown-owner', 'anonymous'])(
    'does not refresh an unverified current account for %s', async (scenario) => {
      routeState.query = { resume_token: 'resume-other', scene: 'image' }
      if (scenario === 'different-owner') authState.setState({ user: { id: 8, balance: 1 } })
      if (scenario === 'anonymous') {
        authState.setState({ user: null, token: null, isAuthenticated: false })
      }
      const resolvedOrder = scenario === 'unknown-owner'
        ? { status: 'COMPLETED', out_trade_no: 'other', paid: true, created_at: '', expires_at: '' }
        : orderFactory('COMPLETED')
      resolveOrderPublicByResumeToken.mockResolvedValue({ data: resolvedOrder })
      const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
      await flushPromises()

      expect(wrapper.text()).toContain('payment.result.success')
      expect(wrapper.text()).toContain('firstUseJourney.completedOrderBody')
      expect(wrapper.text()).not.toContain('firstUseJourney.balanceCreditedBody')
      expect(refreshUser).not.toHaveBeenCalled()
      expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
      expect(window.localStorage.length).toBe(0)
      await wrapper.get('[data-test="continue-setup"]').trigger('click')
      if (scenario === 'anonymous') {
        expect(wrapper.text()).toContain('firstUseJourney.loginToContinue')
        expect(routerPush).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/get-started?scene=image' } })
      }
    },
  )

  it('does not forward arbitrary scene or payment credentials into the guide', async () => {
    routeState.query = { resume_token: 'resume-secret', scene: 'https://evil.example', api_key: 'secret', token: 'secret' }
    resolveOrderPublicByResumeToken.mockResolvedValue({ data: orderFactory('COMPLETED') })
    const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
    await flushPromises()
    await wrapper.get('[data-test="continue-setup"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith({ path: '/get-started', query: {} })
    expect(window.localStorage.length).toBe(0)
  })

  it('does not infer COMPLETED or a next step from query trade_status when the API cannot verify the order', async () => {
    routeState.query = { out_trade_no: 'unverified', trade_status: 'TRADE_SUCCESS', status: 'COMPLETED' }
    verifyOrder.mockRejectedValueOnce(new Error('offline'))
    verifyOrderPublic.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
    await flushPromises()
    expect(wrapper.find('[data-test="funding-completed"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(false)
    expect(refreshUser).not.toHaveBeenCalled()
    expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('payment.result.success')
  })

  it.each(['empty', 'expired', 'future', 'other-user'])(
    'keeps the completed order but does not label %s subscriptions as usable', async (scenario) => {
      routeState.query = { resume_token: 'resume-subscription-unusable' }
      resolveOrderPublicByResumeToken.mockResolvedValue({ data: { ...orderFactory('COMPLETED'), order_type: 'subscription' } })
      const subscription = usableSubscription()
      if (scenario === 'expired') subscription.expires_at = '2000-01-01T00:00:00Z'
      if (scenario === 'future') subscription.starts_at = '2098-01-01T00:00:00Z'
      if (scenario === 'other-user') subscription.user_id = 8
      fetchActiveSubscriptions.mockResolvedValueOnce(scenario === 'empty' ? [] : [subscription])
      const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
      await flushPromises()

      expect(wrapper.text()).toContain('payment.result.success')
      expect(wrapper.text()).toContain('firstUseJourney.accountRefreshPendingBody')
      expect(wrapper.text()).not.toContain('firstUseJourney.subscriptionCreditedBody')
      expect(wrapper.find('[data-test="continue-setup"]').exists()).toBe(true)
    },
  )

  it.each(['switch-back', 'logout-login'])(
    'does not promote a late profile response after %s into a ready entitlement', async (scenario) => {
      routeState.query = { resume_token: 'resume-deferred-profile' }
      resolveOrderPublicByResumeToken.mockResolvedValue({ data: orderFactory('COMPLETED') })
      let resolveProfile!: (profile: { id: number; balance: number }) => void
      refreshUser.mockImplementationOnce(() => new Promise((resolve) => { resolveProfile = resolve }))
      const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
      await flushPromises()
      expect(refreshUser).toHaveBeenCalledOnce()
      if (scenario === 'switch-back') {
        authState.setState({ user: { id: 8, balance: 0 }, token: 'session-b' })
      } else {
        authState.setState({ user: null, token: null, isAuthenticated: false })
      }
      await flushPromises()
      expect(wrapper.text()).not.toContain('firstUseJourney.balanceCreditedBody')
      authState.setState({ user: { id: 9, balance: 0 }, token: 'session-new-a', isAuthenticated: true })
      resolveProfile({ id: 9, balance: 88 })
      await flushPromises()

      expect(wrapper.text()).toContain('payment.result.success')
      expect(wrapper.text()).toContain('firstUseJourney.accountRefreshPendingBody')
      expect(wrapper.text()).not.toContain('firstUseJourney.balanceCreditedBody')
      expect(refreshUser).toHaveBeenCalledOnce()
      expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
    },
  )

  it('does not promote a late subscription response after switching away and back to the paying account', async () => {
    routeState.query = { resume_token: 'resume-deferred-subscription' }
    resolveOrderPublicByResumeToken.mockResolvedValue({ data: { ...orderFactory('COMPLETED'), order_type: 'subscription' } })
    let resolveSubscriptions!: (subscriptions: ReturnType<typeof usableSubscription>[]) => void
    fetchActiveSubscriptions.mockImplementationOnce(() => new Promise((resolve) => { resolveSubscriptions = resolve }))
    const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
    await flushPromises()
    authState.setState({ user: { id: 8, balance: 0 }, token: 'session-b' })
    await flushPromises()
    authState.setState({ user: { id: 9, balance: 0 }, token: 'session-new-a' })
    resolveSubscriptions([usableSubscription()])
    await flushPromises()
    expect(wrapper.text()).toContain('firstUseJourney.accountRefreshPendingBody')
    expect(wrapper.text()).not.toContain('firstUseJourney.subscriptionCreditedBody')
    expect(fetchActiveSubscriptions).toHaveBeenCalledOnce()
  })

  it('accepts a valid profile response across token rotation while the same account stays signed in', async () => {
    routeState.query = { resume_token: 'resume-token-rotation' }
    resolveOrderPublicByResumeToken.mockResolvedValue({ data: orderFactory('COMPLETED') })
    let resolveProfile!: (profile: { id: number; balance: number }) => void
    refreshUser.mockImplementationOnce(() => new Promise((resolve) => { resolveProfile = resolve }))
    const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
    await flushPromises()
    authState.setState({ token: 'rotated-token-a' })
    resolveProfile({ id: 9, balance: 88 })
    await flushPromises()
    expect(wrapper.text()).toContain('firstUseJourney.balanceCreditedBody')
    expect(wrapper.text()).not.toContain('firstUseJourney.accountRefreshPendingBody')
    expect(refreshUser).toHaveBeenCalledOnce()
  })

  it('does not begin entitlement reads after the result page unmounts during order verification', async () => {
    routeState.query = { resume_token: 'resume-after-unmount' }
    let resolveOrder!: (result: { data: ReturnType<typeof orderFactory> }) => void
    resolveOrderPublicByResumeToken.mockImplementationOnce(() => new Promise((resolve) => { resolveOrder = resolve }))
    const wrapper = mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
    await flushPromises()
    wrapper.unmount()
    resolveOrder({ data: orderFactory('COMPLETED') })
    await flushPromises()
    expect(refreshUser).not.toHaveBeenCalled()
    expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
    expect(routerPush).not.toHaveBeenCalled()
  })
})
