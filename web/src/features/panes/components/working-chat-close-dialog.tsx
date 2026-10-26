import { useStore } from 'zustand'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { cancelCloseRequest, confirmCloseRequest } from '@/features/panes/lib/guarded-close'
import { closeRequestStore } from '@/features/panes/stores/close-request-store'

/** The question before a close would stop a chat that is mid-turn. */
export function WorkingChatCloseDialog() {
  const request = useStore(closeRequestStore, (s) => s.request)
  if (!request) return null

  const many = request.working > 1
  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open) cancelCloseRequest()
      }}
    >
      <AlertDialogContent className="max-w-md">
        <AlertDialogHeader>
          <AlertDialogTitle>
            {many ? `${request.working} chats are still working` : 'This chat is still working'}
          </AlertDialogTitle>
          <AlertDialogDescription>
            Closing stops {many ? 'them' : 'it'} mid-turn. The conversation is kept and picks up
            again when you reopen {many ? 'them' : 'it'}.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <Button variant="ghost" onClick={cancelCloseRequest} autoFocus>
            Keep working
          </Button>
          <Button variant="destructive" onClick={confirmCloseRequest}>
            Close anyway
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
