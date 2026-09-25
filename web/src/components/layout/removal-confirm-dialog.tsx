import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import type { WorkAtRisk } from '@/lib/api'
import type { RemovalEntry } from '@/lib/store/sidebar-removal'

/**
 * The last question before a repo or a project is deleted — or before anything
 * is deleted along with work that exists nowhere else.
 *
 * Everything else in the tray is undone by a clock: a workspace or a folder
 * drains for eight seconds and Keep puts it back. That is a proportionate safety
 * net for one row. It is not one for a repo, which takes every worktree under
 * it, and it is emphatically not one for a project, which takes every repo AND
 * every worktree under each — so those two never run a clock at all. They wait
 * in the tray for an answer, and pressing Remove asks this.
 *
 * It spells the cascade out in words rather than leaving "Remove" to imply it.
 * And when the daemon refused a delete over uncommitted files or unmerged
 * commits, it names each branch and what it would lose, and only an explicit
 * "Delete anyway" sends the delete that destroys them.
 */
export function RemovalConfirmDialog({
  entry,
  onCancel,
  onConfirm,
}: {
  /** The entry awaiting confirmation, or null when nothing is. */
  entry: RemovalEntry | null
  onCancel: () => void
  onConfirm: (entry: RemovalEntry) => void
}) {
  if (!entry) return null

  const noun = NOUNS[entry.kind]
  const atRisk = entry.atRisk
  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open) onCancel()
      }}
    >
      <AlertDialogContent className="max-w-md">
        <AlertDialogHeader>
          <AlertDialogTitle>
            {atRisk
              ? `Delete ${noun} “${entry.label}” and lose work?`
              : `Delete ${noun} “${entry.label}”?`}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {atRisk ? <WorkAtRiskText /> : <CascadeText isProject={entry.kind === 'project'} />}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {atRisk && <WorkAtRiskList atRisk={atRisk} />}
        <AlertDialogFooter>
          {/* Cancel first and focused: this dialog exists to be dismissed more
              often than it is confirmed. */}
          <Button variant="ghost" onClick={onCancel} autoFocus>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => onConfirm(entry)}>
            {atRisk ? 'Delete anyway' : `Delete ${noun}`}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

const NOUNS: Record<RemovalEntry['kind'], string> = {
  project: 'space',
  repo: 'repository',
  workspace: 'workspace',
  chat: 'chat',
  folder: 'folder',
}

function CascadeText({ isProject }: { isProject: boolean }) {
  if (isProject) {
    return (
      <>
        <strong className="text-foreground">
          All repositories and workspaces inside it will be deleted.
        </strong>{' '}
        Every worktree Crowbar created for them is removed from disk. Repositories you imported are
        unregistered — the folders you originally pointed Crowbar at are left where they are.
      </>
    )
  }
  return (
    <>
      <strong className="text-foreground">
        All workspaces in this repository will be deleted.
      </strong>{' '}
      Every worktree Crowbar created for them is removed from disk. The folder you originally
      imported is left where it is.
    </>
  )
}

function WorkAtRiskText() {
  return (
    <>
      <strong className="text-foreground">
        This work exists nowhere else — not on another branch, a remote or a tag.
      </strong>{' '}
      Deleting removes it for good. Your own checkout is never touched.
    </>
  )
}

function WorkAtRiskList({ atRisk }: { atRisk: readonly WorkAtRisk[] }) {
  return (
    <ul aria-label="Work that would be lost" className="flex flex-col gap-1 text-sm">
      {atRisk.map((w) => (
        <li key={w.workspaceId} className="flex flex-col">
          <span className="font-mono text-foreground">{w.branch}</span>
          <span className="text-muted-foreground">{lossSummary(w)}</span>
        </li>
      ))}
    </ul>
  )
}

function lossSummary(w: WorkAtRisk): string {
  const parts: string[] = []
  if (w.uncommittedFiles > 0) parts.push(count(w.uncommittedFiles, 'uncommitted file'))
  if (w.unmergedCommits > 0) parts.push(count(w.unmergedCommits, 'unmerged commit'))
  return parts.join(' · ')
}

function count(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? '' : 's'}`
}
