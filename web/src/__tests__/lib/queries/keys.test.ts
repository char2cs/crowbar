import { queryKeys } from '@/lib/queries/keys'

describe('queryKeys', () => {
  describe('workspaces', () => {
    it('all is the root', () => {
      expect(queryKeys.workspaces.all).toEqual(['workspaces'])
    })
    it('list extends all', () => {
      expect(queryKeys.workspaces.list()).toEqual(['workspaces', 'list'])
    })
    it('detail extends all with id', () => {
      expect(queryKeys.workspaces.detail('ws-1')).toEqual(['workspaces', 'ws-1'])
    })
  })

  describe('chats', () => {
    it('messages key includes chatId', () => {
      expect(queryKeys.chats.messages('chat-abc')).toEqual(['chats', 'chat-abc', 'messages'])
    })
    it('byWorkspace key includes workspaceId', () => {
      expect(queryKeys.chats.byWorkspace('ws-1')).toEqual(['chats', 'workspace', 'ws-1'])
    })
  })

  describe('git', () => {
    it('status key includes workspaceId', () => {
      expect(queryKeys.git.status('ws-1')).toEqual(['git', 'ws-1', 'status'])
    })
  })

  describe('files', () => {
    it('tree key includes workspaceId and path', () => {
      expect(queryKeys.files.tree('ws-1', '/src')).toEqual(['files', 'ws-1', 'tree', '/src'])
    })
  })

  it('workspace detail and list both start with workspaces.all', () => {
    const all = queryKeys.workspaces.all
    expect(queryKeys.workspaces.list().slice(0, all.length)).toEqual([...all])
    expect(queryKeys.workspaces.detail('x').slice(0, all.length)).toEqual([...all])
  })
})
