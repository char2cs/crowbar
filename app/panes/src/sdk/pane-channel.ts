/** The text channel the shell injects into every pane page (`window.crowbar`). Text only: bulk and binary data go over the `crowbar://` protocol and the relay socket, never this channel. */
export interface PaneChannel {
  post(text: string): void
  /** Set once; text the shell sent before it was set is delivered on assignment. */
  onmessage: ((text: string) => void) | null
}
