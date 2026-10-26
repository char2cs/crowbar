import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { useEffect, useEffectEvent } from 'react'
import { Button } from '@/components/ui/button'
import { hasNativeChrome, nativeConfirm } from '@/lib/native-chrome'
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
  if (hasNativeChrome()) {
    return <NativeRemovalConfirm entry={entry} onCancel={onCancel} onConfirm={onConfirm} />
  }

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
          <AlertDialogTitle>{titleOf(entry)}</AlertDialogTitle>
          <AlertDialogDescription>
            <Lead
              text={
                atRisk ? WORK_AT_RISK : entry.kind === 'project' ? PROJECT_CASCADE : REPO_CASCADE
              }
            />
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
            {confirmLabelOf(entry)}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function titleOf(entry: RemovalEntry): string {
  const noun = NOUNS[entry.kind]
  return entry.atRisk
    ? `Delete ${noun} “${entry.label}” and lose work?`
    : `Delete ${noun} “${entry.label}”?`
}

function confirmLabelOf(entry: RemovalEntry): string {
  return entry.atRisk ? 'Delete anyway' : `Delete ${NOUNS[entry.kind]}`
}

// One alert per held entry, however often the effect asking for it re-runs while it is up.
const asked = new Map<string, Promise<boolean>>()

function askNatively(entry: RemovalEntry): Promise<boolean> {
  const pending = asked.get(entry.entryId)
  if (pending) return pending
  const lost = entry.atRisk?.map((w) => `${w.branch}: ${lossSummary(w)}`) ?? []
  const lead = entry.atRisk
    ? WORK_AT_RISK
    : entry.kind === 'project'
      ? PROJECT_CASCADE
      : REPO_CASCADE
  const answer = nativeConfirm({
    title: titleOf(entry),
    message: [lead.strong, lead.rest, ...lost].join('\n'),
    confirmLabel: confirmLabelOf(entry),
    cancelLabel: 'Cancel',
    destructive: true,
  }).finally(() => asked.delete(entry.entryId))
  asked.set(entry.entryId, answer)
  return answer
}

// The phone asks the same question in a native alert, over whichever sheet is on screen.
function NativeRemovalConfirm({
  entry,
  onCancel,
  onConfirm,
}: {
  entry: RemovalEntry
  onCancel: () => void
  onConfirm: (entry: RemovalEntry) => void
}) {
  const answer = useEffectEvent((yes: boolean) => {
    if (yes) onConfirm(entry)
    else onCancel()
  })
  useEffect(() => {
    let live = true
    void askNatively(entry).then((yes) => {
      if (live) answer(yes)
    })
    return () => {
      live = false
    }
  }, [entry])
  return null
}

const NOUNS: Record<RemovalEntry['kind'], string> = {
  project: 'space',
  repo: 'repository',
  workspace: 'workspace',
  chat: 'chat',
  folder: 'folder',
}

const PROJECT_CASCADE = {
  strong: 'All repositories and workspaces inside it will be deleted.',
  rest: 'Every worktree Crowbar created for them is removed from disk. Repositories you imported are unregistered — the folders you originally pointed Crowbar at are left where they are.',
}
const REPO_CASCADE = {
  strong: 'All workspaces in this repository will be deleted.',
  rest: 'Every worktree Crowbar created for them is removed from disk. The folder you originally imported is left where it is.',
}
const WORK_AT_RISK = {
  strong: 'This work exists nowhere else — not on another branch, a remote or a tag.',
  rest: 'Deleting removes it for good. Your own checkout is never touched.',
}

function Lead({ text }: { text: { strong: string; rest: string } }) {
  return (
    <>
      <strong className="text-foreground">{text.strong}</strong> {text.rest}
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
