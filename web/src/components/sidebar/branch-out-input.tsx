import { useId, useState } from 'react'
import { cn } from '@/lib/utils'
import { CREATE_ROW_PLACEHOLDER, ROW_SUBLABEL } from '@/components/layout/workspace-row-base'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { rowSubStyle, useRowMetrics } from '@/components/sidebar/lib/row-metrics'
import { branchOutChat } from '@/features/agent/api/agent-chat-commands'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { userWords } from '@/lib/errors/report'

const stop = () => treeUiStore.getState().stopBranching()

/** A thread's title turned, in place, into the input naming the branch Make a branch cuts: Enter
 *  asks the daemon, which refuses a name it cannot take while the input stays open to fix it. */
export function BranchOutInput({ row }: { row: SidebarRow }) {
  const metrics = useRowMetrics()
  const errorId = useId()
  const [error, setError] = useState<string | null>(null)
  const confirm = async (branch: string) => {
    if (!row.workspaceId) return stop()
    try {
      await branchOutChat(row.workspaceId, row.id, branch)
    } catch (err) {
      setError(userWords(err) ?? "Couldn't make the branch")
      throw err
    }
    stop()
  }
  return (
    <span className="flex min-w-0 flex-1 flex-col justify-center">
      <InlineRenameInput
        defaultValue=""
        mono
        placeholder={CREATE_ROW_PLACEHOLDER}
        cancelOnBlur
        errorId={error ? errorId : undefined}
        className="flex-none"
        onConfirm={confirm}
        onCancel={stop}
      />
      {error && (
        <span
          id={errorId}
          role="alert"
          className={cn(ROW_SUBLABEL, 'text-destructive')}
          style={rowSubStyle(metrics)}
          title={error}
        >
          {error}
        </span>
      )}
    </span>
  )
}
