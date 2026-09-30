import { describe, expect, it } from 'vitest'
import type { Group, UserSubscription } from '@/types'
import { hasUsableSubscription, resolveFundingPhase } from './funding-readiness'

const now = Date.parse('2026-09-30T04:00:00Z')
const validSubscription = {
  status: 'active', group_id: 10,
  starts_at: '2026-09-01T00:00:00Z', expires_at: '2026-10-01T00:00:00Z',
  group: { id: 10, status: 'active' } as Group
} as UserSubscription

describe('funding readiness', () => {
  it.each([
    [{ runMode: null, balance: null, hasActiveSubscription: null }, 'unknown'],
    [{ runMode: 'simple', balance: 0, hasActiveSubscription: null }, 'simple'],
    [{ runMode: 'standard', balance: 2, hasActiveSubscription: null }, 'balance'],
    [{ runMode: 'standard', balance: 0, hasActiveSubscription: true }, 'subscription'],
    [{ runMode: 'standard', balance: 0, hasActiveSubscription: false }, 'empty'],
    [{ runMode: 'standard', balance: 0, hasActiveSubscription: null }, 'unknown'],
    [{ runMode: 'standard', balance: null, hasActiveSubscription: false }, 'unknown'],
    [{ runMode: 'standard', balance: Number.NaN, hasActiveSubscription: false }, 'unknown']
  ] as const)('resolves confirmed facts %#', (facts, expected) => {
    expect(resolveFundingPhase(facts)).toBe(expected)
  })

  it('accepts a current subscription in an active attached group', () => {
    expect(hasUsableSubscription(validSubscription, now)).toBe(true)
  })

  it.each([
    { status: 'expired' },
    { starts_at: '2026-10-01T00:00:00Z' },
    { starts_at: 'not a date' },
    { expires_at: '2026-09-30T04:00:00Z' },
    { expires_at: '2026-09-01T00:00:00Z' },
    { expires_at: null },
    { expires_at: 'not a date' },
    { group: undefined },
    { group: { id: 10, status: 'inactive' } },
    { group: { id: 11, status: 'active' } }
  ])('rejects unusable subscription %#', (override) => {
    expect(hasUsableSubscription({ ...validSubscription, ...override } as UserSubscription, now)).toBe(false)
  })
})
