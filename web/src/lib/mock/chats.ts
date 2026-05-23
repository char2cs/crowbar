import type { ChatMessage } from '@/lib/types'

export interface MockChat {
  id: string
  title: string
  age: string
  messages: ChatMessage[]
}

const INITIAL_CHATS: MockChat[] = [
  {
    id: 'c1',
    title: 'Architecture decisions',
    age: '2h',
    messages: [
      {
        id: 'a1', role: 'user',
        content: 'How should we structure the architecture across all three Rabbyte products?',
        authorName: 'Mateo', authorInitials: 'MU', timestamp: '2h ago',
      },
      {
        id: 'a2', role: 'assistant',
        content: 'The three products share a user identity layer, so a shared auth service is the right foundation. crowbar handles agent orchestration, quiver.core is the shared library, and quiver.desktop consumes both.',
        authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
        timestamp: '2h ago · 6.1s', toolCalls: 2, durationSec: 6.1,
      },
    ],
  },
  {
    id: 'c2',
    title: 'Auth strategy across services',
    age: '5d',
    messages: [],
  },
]

const store = new Map<string, MockChat>(
  INITIAL_CHATS.map(c => [c.id, c]),
)

export function getMockChat(id: string): MockChat | undefined {
  return store.get(id)
}

export function getAllMockChats(): MockChat[] {
  return Array.from(store.values())
}

export function createMockChat(): MockChat {
  const id = `c-${Date.now()}`
  const chat: MockChat = { id, title: 'New chat', age: 'just now', messages: [] }
  store.set(id, chat)
  return chat
}

export function deleteMockChat(id: string): void {
  store.delete(id)
}
