import { describe, expect, it } from 'vitest'
import { VOTE_AI_LOGO_URL, isVoteAiPublicPath } from '../branding'

describe('Vote AI branding', () => {
  it('uses the isolated Vote AI logo asset', () => {
    expect(VOTE_AI_LOGO_URL).toBe('/vote-ai-logo.png')
  })

  it.each(['/home', '/pricing', '/docs', '/docs/quick-start'])('recognizes %s as a branded route', path => {
    expect(isVoteAiPublicPath(path)).toBe(true)
  })

  it.each(['/', '/login', '/admin/dashboard'])('leaves %s on the configured site branding', path => {
    expect(isVoteAiPublicPath(path)).toBe(false)
  })
})
