export const VOTE_AI_LOGO_URL = '/vote-ai-logo.png'

export function isVoteAiPublicPath(path: string): boolean {
  return path === '/home' || path === '/pricing' || path === '/docs' || path.startsWith('/docs/')
}
