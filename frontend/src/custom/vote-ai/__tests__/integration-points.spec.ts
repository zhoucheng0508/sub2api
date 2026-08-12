import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const frontendRoot = resolve(dir, '../../../..')
const homeSource = readFileSync(resolve(frontendRoot, 'src/views/HomeView.vue'), 'utf8')
const routerSource = readFileSync(resolve(frontendRoot, 'src/router/index.ts'), 'utf8')
const tailwindSource = readFileSync(resolve(frontendRoot, 'tailwind.config.js'), 'utf8')
const appSource = readFileSync(resolve(frontendRoot, 'src/App.vue'), 'utf8')
const sidebarSource = readFileSync(resolve(frontendRoot, 'src/components/layout/AppSidebar.vue'), 'utf8')
const riskControlSource = readFileSync(resolve(frontendRoot, 'src/views/admin/RiskControlView.vue'), 'utf8')

describe('Vote AI upstream integration points', () => {
  it('keeps the isolated branded homepage attached to the official shell', () => {
    expect(homeSource).toContain('CUSTOM(VOTE-AI-HOME)')
    expect(homeSource).toContain("@/custom/vote-ai/views/VoteAiHome.vue")
  })

  it('redirects legacy pricing links to the official model plaza', () => {
    expect(routerSource).toContain("path: '/pricing'")
    expect(routerSource).toContain("redirect: '/model-plaza'")
    expect(routerSource).not.toContain("@/custom/vote-ai/views/PricingView.vue")
  })

  it('keeps documentation on its isolated route component', () => {
    expect(routerSource).toContain('CUSTOM(VOTE-AI-DOCS)')
    expect(routerSource).toContain("@/custom/vote-ai/views/DocsView.vue")
  })

  it('keeps the branded theme marker searchable', () => {
    expect(tailwindSource).toContain('CUSTOM(VOTE-AI-THEME)')
  })

  it('keeps the public-route favicon integration searchable', () => {
    expect(appSource).toContain('CUSTOM(VOTE-AI-BRANDING)')
    expect(appSource).toContain("@/custom/vote-ai/branding")
  })

  it('keeps the console brand fallback searchable', () => {
    expect(sidebarSource).toContain('CUSTOM(VOTE-AI-BRANDING)')
    expect(sidebarSource).toContain('siteLogo || VOTE_AI_LOGO_URL')
  })

  it('keeps AI content audit isolated at the official risk-control integration point', () => {
    expect(riskControlSource).toContain('CUSTOM(VOTE-AI-AI-AUDIT)')
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/AuditProviderSelector.vue")
    expect(riskControlSource).toContain('providerDrafts')
    expect(riskControlSource).toContain('@close="closeSettings"')
    expect(riskControlSource).toContain('applyConfig(savedConfigSnapshot.value)')
    expect(riskControlSource).toContain('audit_provider: configForm.audit_provider')
    expect(riskControlSource).toContain('ai_confidence_threshold:')
    expect(riskControlSource).toContain("configForm.audit_provider === 'ai_chat' ? [] : moderationTestImages.value")
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/RecommendedPromptControl.vue")
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/ModerationPerformanceSettings.vue")
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/IncrementalAuditSettings.vue")
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/AuditStageDiagnostics.vue")
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/ModerationTestOutcome.vue")
    expect(riskControlSource).toContain('ai_synchronous_budget_ms:')
    expect(riskControlSource).toContain('ai_incremental_audit_enabled:')
    expect(riskControlSource).not.toContain('你是 API 中转网关的内容安全分类器')
  })

  it('keeps user and account audit scopes isolated at the risk-control integration point', () => {
    expect(riskControlSource).toContain('CUSTOM(VOTE-AI-RISK-SCOPE)')
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/ScopeEntitySelector.vue")
    expect(riskControlSource).toContain('user_filter: buildUserFilterPayload()')
    expect(riskControlSource).toContain('account_filter: buildAccountFilterPayload()')
  })

  it('keeps structured moderation status and guarded unban UI isolated', () => {
    expect(riskControlSource).toContain('CUSTOM(VOTE-AI-RISK-SIDE-EFFECTS)')
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/ModerationAuditStatusBadge.vue")
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/ModerationSideEffectsStatus.vue")
    expect(riskControlSource).toContain("@/custom/vote-ai/risk-control/ModerationUnbanDialog.vue")
    expect(riskControlSource).toContain("result.risk_state_cleared")
    expect(riskControlSource).toContain('result.warning')
  })
})
