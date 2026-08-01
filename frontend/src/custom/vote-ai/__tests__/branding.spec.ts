import { describe, expect, it } from 'vitest'
import { VOTE_AI_LOGO_URL } from '../branding'

describe('Vote AI branding', () => {
  it('uses the isolated Vote AI logo asset', () => {
    expect(VOTE_AI_LOGO_URL).toBe('/vote-ai-logo.png')
  })
})
