import { createStore } from 'zustand'
import type { PaneClient } from '@/sdk/pane-client'

/** The text of the file pane's document as the shell last sent it; the shell owns the text. */
export interface DocumentSession {
  /** Null until the shell has sent it, and again once the page is parked. */
  text: string | null
}

/** Fed by the shell's `document.text` frames from boot on, so none is missed before React mounts. */
export const documentSession = createStore<DocumentSession>()(() => ({ text: null }))

/** Feeds `documentSession` from `client` until the page is parked or destroyed. */
export function followDocumentText(client: Pick<PaneClient, 'onDocumentText' | 'onLifecycle'>) {
  client.onDocumentText((text) => documentSession.setState({ text }))
  client.onLifecycle(({ event }) => {
    if (event === 'park' || event === 'destroy') documentSession.setState({ text: null })
  })
}
