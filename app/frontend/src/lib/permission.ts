// A session's permission mode (SessionInfo.permission, read by the daemon
// from its command line) as the UI names it.
import { t } from './i18n.svelte'

export type PermissionMode = 'bypass' | 'auto-edit' | 'plan'

function known(mode: string | undefined): PermissionMode | null {
  return mode === 'bypass' || mode === 'auto-edit' || mode === 'plan' ? mode : null
}

/** Short label ("auto-approve"), or "" for the agent's default. */
export function permLabel(mode: string | undefined): string {
  switch (known(mode)) {
    case 'bypass':
      return t('perm.bypass')
    case 'auto-edit':
      return t('perm.auto-edit')
    case 'plan':
      return t('perm.plan')
  }
  return ''
}

/** One sentence on what the mode lets the agent do, or "". */
export function permHelp(mode: string | undefined): string {
  switch (known(mode)) {
    case 'bypass':
      return t('perm.bypass.help')
    case 'auto-edit':
      return t('perm.auto-edit.help')
    case 'plan':
      return t('perm.plan.help')
  }
  return ''
}
