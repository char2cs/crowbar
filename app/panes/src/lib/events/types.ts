export type DaemonEvent =
  | { type: 'workspace:updated' }
  | { type: 'chat:message'; chatId: string }
  | { type: 'git:changed'; workspaceId: string }
  | { type: 'file:changed'; workspaceId: string; path: string }
  | { type: 'daemon:status'; state: 'starting' | 'ready' | 'degraded' | 'reconnecting' }
