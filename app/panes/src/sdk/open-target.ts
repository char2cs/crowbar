/** The file a shell-opened editor pane shows: workspace-relative (`src/a.ts`) or absolute for a host document, kept as a tab or opened as the group's preview. */
export interface OpenTarget {
  file: string
  mode: 'kept' | 'preview'
}
