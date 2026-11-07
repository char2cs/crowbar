import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { performBranchOut } from '@/components/sidebar/lib/drop-actions'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { treeUiStore, useTreeUi } from '@/features/workspace/tree/tree-ui-store'

/** The branch-name input a thread's label turns into on Make a branch: Enter cuts the branch. */
export function BranchOutInput({ row }: { row: SidebarRow }) {
  const name = useTreeUi((s) => s.branching?.name ?? '')
  const stop = () => treeUiStore.getState().stopBranching()
  return (
    <InlineRenameInput
      defaultValue={name}
      mono
      onConfirm={(branch) => {
        stop()
        void performBranchOut(row, branch)
      }}
      onCancel={stop}
    />
  )
}
