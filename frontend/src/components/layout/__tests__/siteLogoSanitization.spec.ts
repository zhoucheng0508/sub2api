import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const sidebarSource = readFileSync(resolve(dir, '../AppSidebar.vue'), 'utf8')
const authLayoutSource = readFileSync(resolve(dir, '../AuthLayout.vue'), 'utf8')
const homeViewSource = readFileSync(resolve(dir, '../../../views/HomeView.vue'), 'utf8')
const docsViewSource = readFileSync(resolve(dir, '../../../custom/vote-ai/views/DocsView.vue'), 'utf8')
const keyUsageViewSource = readFileSync(resolve(dir, '../../../views/KeyUsageView.vue'), 'utf8')

describe('site_logo sanitization', () => {
  it('AppSidebar imports sanitizeUrl and applies it to siteLogo', () => {
    expect(sidebarSource).toContain("import { sanitizeUrl } from '@/utils/url'")
    expect(sidebarSource).toContain('sanitizeUrl(appStore.siteLogo')
  })

  it('console and authentication layouts use the isolated Vote AI fallback', () => {
    for (const src of [sidebarSource, authLayoutSource]) {
      expect(src).toContain("import { VOTE_AI_LOGO_URL } from '@/custom/vote-ai/branding'")
      expect(src).toContain('siteLogo || VOTE_AI_LOGO_URL')
    }
  })

  it('HomeView applies sanitizeUrl to siteLogo', () => {
    expect(homeViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo')
  })

  it('DocsView loads the complete admin list immediately for an authenticated admin', () => {
    expect(docsViewSource).toContain("watch(isAdmin, admin => { if (!saving.value) loadDocs(admin) }, { immediate: true })")
    expect(docsViewSource).not.toContain('loadDocs(false)')
  })

  it('DocsView prefers the configured logo and falls back to the Vote AI asset', () => {
    expect(docsViewSource).toContain("import { VOTE_AI_LOGO_URL } from '../branding'")
    expect(docsViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo')
    expect(docsViewSource).toContain(':src="siteLogo || VOTE_AI_LOGO_URL"')
  })

  it('KeyUsageView applies sanitizeUrl to siteLogo', () => {
    expect(keyUsageViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo')
  })

  it('configurable official page logos pass allowRelative and allowDataUrl options', () => {
    for (const src of [sidebarSource, homeViewSource, docsViewSource, keyUsageViewSource]) {
      expect(src).toContain('allowRelative: true')
      expect(src).toContain('allowDataUrl: true')
    }
  })
})
