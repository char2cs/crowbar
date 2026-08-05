import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import type { RemovalEntry } from '@/lib/store/sidebar-removal'

/**
 * The last question before a repo or a project is deleted.
 *
 * Everything else in the tray is undone by a clock: a workspace or a folder
 * drains for eight seconds and Keep puts it back. That is a proportionate safety
 * net for one row. It is not one for a repo, which takes every worktree under
 * it, and it is emphatically not one for a project, which takes every repo AND
 * every worktree under each — so those two never run a clock at all. They wait
 * in the tray for an answer, and pressing Remove asks this.
 *
 * It spells the cascade out in words rather than leaving "Remove" to imply it.
 * The tray row beside it says the name and a count; what it cannot say, in
 * 36px, is that the count is repositories and every branch inside them.
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

  const isProject = entry.kind === 'project'
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
            Delete {isProject ? 'project' : 'repository'} “{entry.label}”?
          </AlertDialogTitle>
          <AlertDialogDescription>
            {isProject ? (
              <>
                <strong className="text-foreground">
                  All repositories and workspaces inside it will be deleted.
                </strong>{' '}
                Every worktree Crowbar created for them is removed from disk. Repositories you
                imported are unregistered — the folders you originally pointed Crowbar at are left
                where they are.
              </>
            ) : (
              <>
                <strong className="text-foreground">
                  All workspaces in this repository will be deleted.
                </strong>{' '}
                Every worktree Crowbar created for them is removed from disk. The folder you
                originally imported is left where it is.
              </>
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          {/* Cancel first and focused: this dialog exists to be dismissed more
              often than it is confirmed. */}
          <Button variant="ghost" onClick={onCancel} autoFocus>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => onConfirm(entry)}>
            Delete {isProject ? 'project' : 'repository'}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
