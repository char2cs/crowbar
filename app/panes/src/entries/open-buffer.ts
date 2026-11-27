import type { DocumentUnavailable } from '@/features/panes/types/pane-content'

export interface OpenBuffer {
  (wsId: string, saved: unknown, refuse: { (reason: DocumentUnavailable): void }): Promise<unknown>
}
