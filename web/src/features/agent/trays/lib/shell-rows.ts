import type { ShellItem } from '@/features/agent/stores/lib/transcript-items'
import type { TrayRow } from '@/features/agent/trays/lib/tray-rows'

const lastLine = (text: string | undefined) => text?.trimEnd().split('\n').pop() ?? ''

function ending(shell: ShellItem): string {
  if (shell.status === 'stopped') return 'Stopped'
  const code = shell.exitCode === undefined ? '' : `Exit ${shell.exitCode}`
  return code || (shell.status === 'failed' ? 'Failed' : 'Finished')
}

function hint(shell: ShellItem): string {
  const timeout = shell.timeoutMs ? `Stops after ${Math.round(shell.timeoutMs / 1000)}s` : ''
  return [
    shell.command,
    shell.cwd && `in ${shell.cwd}`,
    shell.ref && `process ${shell.ref}`,
    shell.persistent ? 'Keeps watching' : timeout,
    shell.summary,
    shell.tail,
  ]
    .filter(Boolean)
    .join('\n')
}

/** A background command or monitor as a tray row; once ended it says how and shows no clock. */
export function shellRow(shell: ShellItem): TrayRow {
  const running = shell.status === 'running'
  const where = shell.label ? shell.command : (shell.cwd ?? '')
  const detail = running
    ? shell.stopping
      ? 'Stopping…'
      : lastLine(shell.tail) || where
    : [ending(shell), shell.summary || lastLine(shell.tail)].filter(Boolean).join(' · ')
  return {
    id: shell.id,
    kind: shell.shellKind,
    shellId: shell.shellId,
    stopping: shell.stopping,
    label: shell.label || shell.command || (shell.shellKind === 'monitor' ? 'Monitor' : 'Shell'),
    detail,
    hint: hint(shell),
    status: running ? 'running' : shell.status === 'finished' ? 'done' : shell.status,
    startedAt: shell.startedAt,
    endedAt: shell.endedAt,
  }
}
