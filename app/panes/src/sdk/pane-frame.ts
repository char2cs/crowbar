import type { PaneParams } from './pane-params'

/** The frames a pane sends; each is wrapped as `{ sdk: 1, ...frame }`. */
export type PaneFrame =
  | { kind: 'pane.register'; typeId: string; sdkVersion: string }
  | { kind: 'pane.open'; typeId: string; params: PaneParams }
  | { kind: 'pane.setTitle'; title: string }
  | { kind: 'pane.setState'; state: unknown }
  | { kind: 'request'; id: number; method: string; params: unknown }
  | { kind: 'subscribe'; id: number; event: string }
  | { kind: 'unsubscribe'; id: number }
  | { kind: 'command.run'; command: string }
  | { kind: 'settings.get'; id: number; key: string }
  | { kind: 'settings.subscribe'; id: number; key: string }
