import type { Group } from '@/types'
import type { CcSwitchAppType } from '@/utils/ccswitchImport'

// Automatic import support of the existing generators, not upstream capability.
// Provider-specific/routed combinations retain manual configuration.
export function supportsAutomaticConfig(group: Pick<Group, 'platform' | 'allow_messages_dispatch' | 'claude_code_only'> | undefined, app: CcSwitchAppType): boolean {
  if (group?.claude_code_only && app !== 'claude') return false
  const platform = group?.platform
  switch (app) {
    case 'codex': return platform === 'openai'
    case 'claude': return platform === 'anthropic' || platform === 'antigravity' || (platform === 'openai' && group?.allow_messages_dispatch === true)
    case 'gemini': return platform === 'gemini' || platform === 'antigravity'
    case 'grokbuild': return platform === 'grok'
    case 'opencode':
    case 'openclaw':
    case 'hermes': return platform === 'openai' || platform === 'grok'
    default: return false
  }
}
