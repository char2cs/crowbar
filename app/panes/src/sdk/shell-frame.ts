import type { Breakpoint } from './breakpoint'
import type { FileDrop } from './file-drop'
import type { LifecycleMessage } from './lifecycle-message'
import type { PairedHost } from './paired-host'
import type { ReviewViewedFrame } from './review-viewed-frame'
import type { RpcFailure } from './rpc-failure'
import type { SettingValues } from './setting-values'
import type { ThemeTokens } from './theme-tokens'

export type ShellFrame =
  | ({ kind: 'lifecycle' } & LifecycleMessage)
  | { kind: 'response'; id: number; result?: unknown; error?: RpcFailure }
  | {
      kind: 'extension.event'
      extensionId: string
      spaceId?: string
      name: string
      payload: unknown
    }
  | { kind: 'protocol.error'; message: string }
  | { kind: 'pane.save' }
  | { kind: 'theme'; tokens: ThemeTokens }
  | { kind: 'breakpoints'; cleared: Breakpoint[] }
  | { kind: 'chat.zoom'; percent: number }
  | { kind: 'group'; chatFills: boolean }
  | { kind: 'settings'; version: number; values: SettingValues }
  | { kind: 'hosts'; hosts: PairedHost[] }
  | ({ kind: 'review.viewed' } & ReviewViewedFrame)
  | { kind: 'review.reveal'; path: string }
  | { kind: 'document.text'; text: string }
  | ({ kind: 'files.dropped' } & FileDrop)
