import { createStore } from 'zustand'
import type { PaneClient } from '@/sdk/pane-client'

export interface DocumentSession {
  text: string | null
}

export const documentSession = createStore<DocumentSession>()(() => ({ text: null }))

export function followDocumentText(client: Pick<PaneClient, 'onDocumentText' | 'onLifecycle'>) {
  client.onDocumentText((text) => documentSession.setState({ text }))
  client.onLifecycle(({ event }) => {
    if (event === 'park' || event === 'destroy') documentSession.setState({ text: null })
  })
}
