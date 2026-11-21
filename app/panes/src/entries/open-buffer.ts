import type { DocumentUnavailable } from '@/features/panes/types/pane-content'

/** Opens the pane's buffer in the workspace `wsId` into the root pane, from the state the shell saved; a buffer the user just opened whose read is refused is handed to `refuse` instead of opening. */
export interface OpenBuffer {
  (wsId: string, saved: unknown, refuse: { (reason: DocumentUnavailable): void }): Promise<unknown>
}
