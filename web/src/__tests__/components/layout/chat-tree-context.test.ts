import { describe, it, expect, beforeEach, vi } from 'vitest'

vi.mock('@/lib/api/chat', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/chat')>()
  return {
    chatDtoToProjectChat: actual.chatDtoToProjectChat,
    postChat: vi.fn(),
    forkChat: vi.fn(),
    patchChat: vi.fn(),
    deleteChat: vi.fn(),
  }
})

vi.mock('@/components/ui/toast', () => ({
  toast: { error: vi.fn() },
}))

import {
  postChat,
  forkChat,
  patchChat,
  deleteChat as apiDeleteChat,
  type ChatDto,
} from '@/lib/api/chat'
import { toast } from '@/components/ui/toast'
import { useSidebarStore, type ProjectChat } from '@/lib/store/sidebar'
import {
  performCreateChat,
  performForkChat,
  performRenameChat,
  performDeleteChat,
} from '@/components/layout/chat-tree-context'

const dto = (overrides: Partial<ChatDto> = {}): ChatDto => ({
  id: 'backend-id',
  wsId: 'ws1',
  title: 'My chat',
  status: 'idle',
  type: 'chat',
  createdAt: new Date().toISOString(),
  ...overrides,
})

const seeded: ProjectChat[] = [
  { id: 'c1', wsId: 'ws1', title: 'Existing', age: 'now', status: 'idle', type: 'chat' },
]

function chatIds(): string[] {
  return useSidebarStore.getState().chats.map((c) => c.id)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  useSidebarStore.setState({ chats: [...seeded] })
})

describe('performCreateChat', () => {
  it('POSTs /v0/workspaces/:wsId/chats and stores the chat under the backend id', async () => {
    vi.mocked(postChat).mockResolvedValue(dto({ id: 'real-backend-id', title: 'New chat' }))

    await performCreateChat('ws1', 'New chat')

    expect(postChat).toHaveBeenCalledWith('ws1', 'New chat')
    const created = useSidebarStore.getState().chats.find((c) => c.id === 'real-backend-id')
    expect(created).toBeDefined()
    expect(created?.title).toBe('New chat')
    expect(created?.wsId).toBe('ws1')
  })

  it('does not POST when wsId is empty', async () => {
    await performCreateChat('', 'New chat')

    expect(postChat).not.toHaveBeenCalled()
    expect(chatIds()).toEqual(['c1'])
  })

  it('does not add a phantom node when the API call fails', async () => {
    vi.mocked(postChat).mockRejectedValue(new Error('500 boom'))

    await performCreateChat('ws1', 'New chat')

    expect(chatIds()).toEqual(['c1'])
    expect(toast.error).toHaveBeenCalledWith('Failed to create chat', '500 boom')
  })
})

describe('performForkChat', () => {
  it('calls POST /v0/chats/:id/fork and adds the fork with the backend id', async () => {
    vi.mocked(forkChat).mockResolvedValue(dto({ id: 'fork-id', title: 'Existing', parentId: 'c1' }))

    await performForkChat('c1', 'Existing')

    expect(forkChat).toHaveBeenCalledWith('c1')
    expect(patchChat).not.toHaveBeenCalled() // same title — no rename needed
    const fork = useSidebarStore.getState().chats.find((c) => c.id === 'fork-id')
    expect(fork?.parentId).toBe('c1')
  })

  it('renames the fork via PATCH when the typed title differs', async () => {
    vi.mocked(forkChat).mockResolvedValue(dto({ id: 'fork-id', title: 'Existing', parentId: 'c1' }))
    vi.mocked(patchChat).mockResolvedValue(dto({ id: 'fork-id', title: 'My fork', parentId: 'c1' }))

    await performForkChat('c1', 'My fork')

    expect(patchChat).toHaveBeenCalledWith('fork-id', 'My fork')
    const fork = useSidebarStore.getState().chats.find((c) => c.id === 'fork-id')
    expect(fork?.title).toBe('My fork')
  })

  it('does not add a phantom node when the fork call fails', async () => {
    vi.mocked(forkChat).mockRejectedValue(new Error('500 boom'))

    await performForkChat('c1', 'My fork')

    expect(chatIds()).toEqual(['c1'])
    expect(toast.error).toHaveBeenCalledWith('Failed to fork chat', '500 boom')
  })
})

describe('performRenameChat', () => {
  it('calls PATCH /v0/chats/:id and renames the chat on success', async () => {
    vi.mocked(patchChat).mockResolvedValue(dto({ id: 'c1', title: 'Renamed' }))

    await performRenameChat('c1', 'Renamed')

    expect(patchChat).toHaveBeenCalledWith('c1', 'Renamed')
    expect(useSidebarStore.getState().chats[0].title).toBe('Renamed')
  })

  it('leaves the title untouched when the API call fails', async () => {
    vi.mocked(patchChat).mockRejectedValue(new Error('409 conflict'))

    await performRenameChat('c1', 'Renamed')

    expect(useSidebarStore.getState().chats[0].title).toBe('Existing')
    expect(toast.error).toHaveBeenCalledWith('Failed to rename chat', '409 conflict')
  })
})

describe('performDeleteChat', () => {
  it('calls DELETE /v0/chats/:id and removes the chat on success', async () => {
    vi.mocked(apiDeleteChat).mockResolvedValue(undefined)

    await performDeleteChat('c1')

    expect(apiDeleteChat).toHaveBeenCalledWith('c1')
    expect(chatIds()).toEqual([])
  })

  it('does not remove the chat when the API call fails', async () => {
    vi.mocked(apiDeleteChat).mockRejectedValue(new Error('500 boom'))

    await performDeleteChat('c1')

    expect(chatIds()).toEqual(['c1'])
    expect(toast.error).toHaveBeenCalledWith('Failed to delete chat', '500 boom')
  })

  it('is a no-op for unknown chat ids', async () => {
    await performDeleteChat('nope')

    expect(apiDeleteChat).not.toHaveBeenCalled()
    expect(chatIds()).toEqual(['c1'])
  })
})
