import { describe, expect, it } from 'vitest'
import { useFirstTokenMonitor } from '../useFirstTokenMonitor'

describe('account first token monitoring settings', () => {
  it('defaults to disabled, persists changes, and reloads per account', () => {
    const { monitor, apply, load } = useFirstTokenMonitor()
    expect(monitor).toEqual({ enabled: false, thresholdSeconds: 30, anomalyCount: 3, cooldownMinutes: 10 })
    monitor.enabled = true
    monitor.thresholdSeconds = 45
    const credentials: Record<string, unknown> = { api_key: 'existing', model_mapping: { a: 'b' } }
    expect(apply(credentials)).toBe(true)
    expect(credentials.api_key).toBe('existing')
    const revision = credentials.first_token_monitor_revision
    expect(apply(credentials)).toBe(true)
    expect(credentials.first_token_monitor_revision).toBe(revision)
    load()
    expect(monitor.enabled).toBe(false)
    load(credentials)
    expect(monitor.enabled).toBe(true)
    expect(monitor.thresholdSeconds).toBe(45)
    monitor.enabled = false
    apply(credentials)
    expect(credentials.first_token_monitor_revision).not.toBe(revision)
  })

  it('rejects invalid enabled settings without mutating credentials', () => {
    const { monitor, apply } = useFirstTokenMonitor()
    monitor.enabled = true
    for (const value of [0, 1.5, 3601, NaN]) {
      monitor.thresholdSeconds = value
      const credentials = { api_key: 'existing' }
      expect(apply(credentials)).toBe(false)
      expect(credentials).toEqual({ api_key: 'existing' })
    }
  })
})
