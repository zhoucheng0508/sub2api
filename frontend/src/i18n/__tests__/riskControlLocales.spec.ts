import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('risk control locale copy', () => {
  it('describes worker runtime as audit and pre-block record processing', () => {
    expect(zh.admin.riskControl.workerStatusHint).toContain('前置拦截记录任务')
    expect(zh.admin.riskControl.workerStatusHint).not.toContain('异步观察任务')
    expect(en.admin.riskControl.workerStatusHint).toContain('pre-block record tasks')
    expect(en.admin.riskControl.workerStatusHint).not.toContain('observation tasks')
  })

  it('keeps pre-block audit key summary aware of async worker load', () => {
    expect(zh.admin.riskControl.preBlockAPIKeyLoadSummary).toContain('worker：{workerActive} / {workerTotal}')
    expect(en.admin.riskControl.preBlockAPIKeyLoadSummary).toContain('worker: {workerActive} / {workerTotal}')
  })

  it('does not describe pre-block audit key polling as bypassing the worker pool', () => {
    expect(zh.admin.riskControl.preBlockAPIKeyLoadHint).toBe('同步前置拦截直接轮询可用审核 Key。')
    expect(zh.admin.riskControl.preBlockAPIKeyLoadHint).not.toContain('Worker 池')
    expect(en.admin.riskControl.preBlockAPIKeyLoadHint).not.toContain('worker pool')
  })

  it('provides structured audit, notification, and guarded unban copy', () => {
    expect(zh.admin.riskControl.auditStatusSkipped).toBe('已跳过')
    expect(en.admin.riskControl.auditStatusSkipped).toBe('Skipped')
    expect(zh.admin.riskControl.notificationStatus.deduplicated).toContain('合并')
    expect(en.admin.riskControl.notificationStatus.deduplicated).toContain('deduplicated')
    expect(zh.admin.riskControl.unbanModeClearRisk).toContain('推荐')
    expect(en.admin.riskControl.unbanModeClearRisk).toContain('recommended')
    expect(zh.admin.riskControl.unbanNotModerationOwned).toContain('不能')
    expect(en.admin.riskControl.unbanNotModerationOwned).toContain('not disabled by content moderation')
    expect(zh.admin.riskControl.unbanPartialSuccess).toContain('{warning}')
    expect(en.admin.riskControl.unbanPartialSuccess).toContain('{warning}')
    expect(zh.admin.riskControl.retryRiskStateCleanup).toContain('重试')
    expect(en.admin.riskControl.retryRiskStateCleanup).toContain('Retry')
  })
})
