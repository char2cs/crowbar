/** Opens the pane's buffer in the workspace `wsId` into the root pane, from the state the shell saved. */
export interface OpenBuffer {
  (wsId: string, saved: unknown): Promise<unknown>
}
