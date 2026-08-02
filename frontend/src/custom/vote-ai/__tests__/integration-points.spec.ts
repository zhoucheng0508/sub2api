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
})
