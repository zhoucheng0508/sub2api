import { reactive } from 'vue'

export function useFirstTokenMonitor() {
  const monitor = reactive({ enabled: false, thresholdSeconds: 30, anomalyCount: 3, cooldownMinutes: 10 })
  function load(credentials?: Record<string, unknown>) {
    monitor.enabled = credentials?.first_token_monitor_enabled === true
    monitor.thresholdSeconds = Number(credentials?.first_token_monitor_threshold_seconds ?? 30)
    monitor.anomalyCount = Number(credentials?.first_token_monitor_anomaly_count ?? 3)
    monitor.cooldownMinutes = Number(credentials?.first_token_monitor_cooldown_minutes ?? 10)
  }
  function apply(credentials: Record<string, unknown>): boolean {
    const valid = [
      [monitor.thresholdSeconds, 3600], [monitor.anomalyCount, 100], [monitor.cooldownMinutes, 1440]
    ].every(([value, max]) => Number.isInteger(value) && value >= 1 && value <= max)
    if (monitor.enabled && !valid) return false
    if (!valid) {
      monitor.thresholdSeconds = 30
      monitor.anomalyCount = 3
      monitor.cooldownMinutes = 10
    }
    const settings = {
      first_token_monitor_enabled: monitor.enabled,
      first_token_monitor_threshold_seconds: monitor.thresholdSeconds,
      first_token_monitor_anomaly_count: monitor.anomalyCount,
      first_token_monitor_cooldown_minutes: monitor.cooldownMinutes
    }
    if (Object.entries(settings).some(([key, value]) => credentials[key] !== value)) {
      credentials.first_token_monitor_revision = `${Date.now()}-${Math.random().toString(36).slice(2)}`
    }
    Object.assign(credentials, settings)
    return true
  }
  return { monitor, load, apply }
}
