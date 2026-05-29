export const queryKeys = {
  workspaces: {
    all: ['workspaces'] as const,
    list: () => [...queryKeys.workspaces.all, 'list'] as const,
    detail: (id: string) => [...queryKeys.workspaces.all, id] as const,
  },
  chats: {
    byWorkspace: (wsId: string) => ['chats', 'workspace', wsId] as const,
    detail: (id: string) => ['chats', id] as const,
    messages: (id: string) => ['chats', id, 'messages'] as const,
  },
  git: {
    status: (wsId: string) => ['git', wsId, 'status'] as const,
    branches: (wsId: string) => ['git', wsId, 'branches'] as const,
    log: (wsId: string) => ['git', wsId, 'log'] as const,
  },
  files: {
    tree: (wsId: string, path: string) => ['files', wsId, 'tree', path] as const,
  },
} as const
