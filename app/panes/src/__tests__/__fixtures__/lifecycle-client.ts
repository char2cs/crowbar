import type { LifecycleMessage } from '@/sdk/lifecycle-message'
import type { PaneClient } from '@/sdk/pane-client'

type Listener = (message: LifecycleMessage) => void

/** A pane client that hands its lifecycle listeners the messages the test delivers. */
export function lifecycleDriver(): {
  client: PaneClient
  deliver: Listener
} {
  const listeners: Listener[] = []
  const onLifecycle = (listener: Listener) => {
    listeners.push(listener)
    return () => undefined
  }
  const deliver: Listener = (message) => listeners.forEach((listener) => listener(message))
  return { client: { onLifecycle } as unknown as PaneClient, deliver }
}
