import type { LifecycleMessage } from './lifecycle-message'
import type { RpcFailure } from './rpc-failure'
import type { SettingValues } from './setting-values'
import type { ThemeTokens } from './theme-tokens'

/** The frames the shell sends that a pane acts on. */
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
  | { kind: 'settings'; version: number; values: SettingValues }
