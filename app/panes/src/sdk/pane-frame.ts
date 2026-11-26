import type { PaneParams } from './pane-params'
import type { OpenTarget } from './open-target'
import type { ViewedFile } from './viewed-file'
import type { PaneToast } from './pane-toast'

/** The frames a pane sends; each is wrapped as `{ sdk: 1, ...frame }`. */
export type PaneFrame =
  | { kind: 'pane.register'; typeId: string; sdkVersion: string }
  | ({ kind: 'pane.open'; typeId: string; params: PaneParams } & Partial<OpenTarget>)
  | { kind: 'pane.setTitle'; title: string }
  | { kind: 'pane.setParams'; params: PaneParams }
  | { kind: 'pane.setState'; state: unknown }
  | { kind: 'pane.setDirty'; dirty: boolean }
  | { kind: 'pane.openRefused'; title?: string; message: string }
  | { kind: 'request'; id: number; method: string; params: unknown }
  | { kind: 'subscribe'; id: number; event: string }
  | { kind: 'unsubscribe'; id: number }
  | { kind: 'command.run'; command: string }
  | { kind: 'shell.openExternal'; url: string }
  | { kind: 'shell.reveal'; path: string }
  | { kind: 'settings.set'; version: number; key: string; value: unknown }
  | { kind: 'review.setViewed'; files: ViewedFile[]; viewed: boolean }
  | { kind: 'document.edit'; text: string }
  | { kind: 'document.showSource' }
  | { kind: 'document.save' }
  | ({ kind: 'shell.toast' } & PaneToast)
  | { kind: 'shell.toastClose'; key: string }
