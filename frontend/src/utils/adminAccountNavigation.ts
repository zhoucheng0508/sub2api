import type { AdminUsageLog } from '@/types'
import { normalizeAdminGroup } from '@/composables/useRememberedAdminGroup'

export function adminAccountLocation(log: Pick<AdminUsageLog, 'account_id' | 'group_id'>) {
  return {
    path: '/admin/accounts',
    query: {
      account_id: String(log.account_id),
      group: normalizeAdminGroup(log.group_id, 'usage')
    }
  }
}
