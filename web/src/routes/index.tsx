import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { AppShell } from '@/components/layout/AppShell'
import { Sidebar, type Repo, type ProjectChat } from '@/components/layout/Sidebar'
import { ChatView, type ChatMessage, type FlowStep } from '@/components/chat/ChatView'

export const Route = createFileRoute('/')({ component: IndexPage })

const MOCK_CHATS: ProjectChat[] = [
  { id: 'c1', title: 'Architecture decisions', age: '2h' },
  { id: 'c2', title: 'Auth strategy across services', age: '5d' },
]

const MOCK_REPOS: Repo[] = [
  {
    id: 'crowbar',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws3', num: 3, branch: 'feature/app-design', added: 5672, age: '16h ago' },
      { id: 'ws2', num: 2, branch: 'feature/api-backend', added: 27347, deleted: 455, age: '1d ago' },
      { id: 'ws1', num: 1, branch: 'enhancement/scaffold', added: 22892, age: '3d ago' },
    ],
  },
  {
    id: 'quiver-core',
    name: 'quiver.core',
    avatarLabel: 'Q',
    avatarColor: 'bg-emerald-700',
    workspaces: [{ id: 'qc1', branch: 'develop', age: '3d ago' }],
  },
  {
    id: 'quiver-desktop',
    name: 'quiver.desktop',
    avatarLabel: 'Q',
    avatarColor: 'bg-orange-700',
    workspaces: [
      { id: 'qd1', branch: 'develop', age: '6d ago' },
      { id: 'qd2', branch: 'feature/quiver-shell', added: 13485, deleted: 69, age: '3d ago' },
    ],
  },
]

const MOCK_MESSAGES: ChatMessage[] = [
  {
    id: 'm1', role: 'user', content: 'How should we handle auth across crowbar, quiver.core and quiver.desktop?',
    authorName: 'Mateo', authorInitials: 'MU', timestamp: '2h ago',
  },
  {
    id: 'm2', role: 'assistant',
    content: 'Given all three share a user identity, a shared auth service makes the most sense — lightweight Go, token issuance and refresh, each app verifying JWTs locally.',
    authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
    timestamp: '2h ago · 18.3s', toolCalls: 4, durationSec: 18.3,
  },
  {
    id: 'm3', role: 'user', content: "Makes sense. Let's go with that.",
    authorName: 'Mateo', authorInitials: 'MU', timestamp: '2h ago',
  },
]

function IndexPage() {
  const [step, setStep] = useState<FlowStep>('brainstorm')
  const [activeChatId, setActiveChatId] = useState('c1')
  const [messages, setMessages] = useState<ChatMessage[]>(MOCK_MESSAGES)

  const handleSend = (content: string) => {
    setMessages(prev => [...prev, {
      id: `m${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }])
  }

  return (
    <AppShell
      sidebar={
        <Sidebar
          projectName="Rabbyte"
          userInitials="MU"
          chats={MOCK_CHATS}
          repos={MOCK_REPOS}
          activeChatId={activeChatId}
          onChatClick={setActiveChatId}
        />
      }
    >
      <ChatView
        step={step}
        onStepChange={setStep}
        messages={messages}
        onSend={handleSend}
        modelName="Sonnet 4.6"
        tokenPct={12}
        inputPlaceholder="Ask about the Rabbyte project…"
      />
    </AppShell>
  )
}
