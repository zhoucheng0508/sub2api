import type { UserSubscription } from '@/types'

export type FundingPhase = 'unknown' | 'empty' | 'balance' | 'subscription' | 'simple'

export interface FundingFacts {
  runMode: 'standard' | 'simple' | null
  balance: number | null
  hasActiveSubscription: boolean | null
}

/** Only confirmed absence of both entitlements should prompt a user to add funds. */
export function resolveFundingPhase(facts: FundingFacts): FundingPhase {
  if (facts.runMode === 'simple') return 'simple'
  if (facts.runMode !== 'standard') return 'unknown'
  if (facts.balance !== null && Number.isFinite(facts.balance) && facts.balance > 0) return 'balance'
  if (facts.hasActiveSubscription === true) return 'subscription'
  if (facts.balance !== null && Number.isFinite(facts.balance) && facts.hasActiveSubscription === false) {
    return 'empty'
  }
  return 'unknown'
}

/** Validate dates and the attached group instead of trusting the subscription label alone. */
export function hasUsableSubscription(subscription: UserSubscription, now = Date.now()): boolean {
  if (subscription.status !== 'active') return false
  const startsAt = Date.parse(subscription.starts_at)
  if (!Number.isFinite(startsAt) || startsAt > now) return false
  if (!subscription.expires_at) return false
  const expiresAt = Date.parse(subscription.expires_at)
  if (!Number.isFinite(expiresAt) || expiresAt <= now) return false
  return subscription.group?.id === subscription.group_id && subscription.group.status === 'active'
}
