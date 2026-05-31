import type { MarkdownTurn, SlashCommand } from '@/features/markdown-chat/types'

const MOCK_TURNS: Record<string, MarkdownTurn[]> = {
  'ws3:brainstorm': [
    {
      id: 'mt1',
      role: 'user',
      content: 'How should we handle auth across crowbar, quiver.core and quiver.desktop?',
      timestamp: '2026-05-31T10:00:00Z',
      authorName: 'Mateo',
      widgets: [],
    },
    {
      id: 'mt2',
      role: 'agent',
      content: `Given all three share a user identity, a **shared auth service** makes the most sense.

Here are the three options I'd consider:

## Option A — Shared Auth Microservice
<!-- tool-call:{"name":"read_file","args":{"path":"src/auth/token.ts"},"status":"done","output":"export async function refreshToken(..."} -->

Each app delegates auth to a central service. Single source of truth for tokens.

## Option B — Auth SDK
Ship a shared \`@quiver/auth\` package that each app installs. Keeps network hops low.

## Option C — OAuth + PKCE per app
Each app runs its own OAuth flow. Simpler to deploy, harder to revoke globally.

I'd recommend **Option A** for Crowbar specifically since you already have a Go backend planned.`,
      timestamp: '2026-05-31T10:01:00Z',
      authorName: 'Claude',
      widgets: [],
    },
    {
      id: 'mt3',
      role: 'user',
      content: "Makes sense. Let's go with Option A.",
      timestamp: '2026-05-31T10:02:00Z',
      authorName: 'Mateo',
      widgets: [],
    },
  ],
  'ws2:build': [
    {
      id: 'mt4',
      role: 'agent',
      content: `Starting implementation. Here's my plan:

- [ ] Read existing auth files
- [ ] Create token-service skeleton
- [ ] Wire up refresh logic
- [ ] Update error handling
- [ ] Write tests`,
      timestamp: '2026-05-31T09:00:00Z',
      authorName: 'Claude',
      widgets: [],
    },
  ],
}

export function getMockMarkdownTurns(wsId: string, stepId: string): MarkdownTurn[] {
  return MOCK_TURNS[`${wsId}:${stepId}`] ?? []
}

// Placeholder for the provider-supplied slash command list. In production these
// arrive from the AI provider; this mock stands in until that feed is wired.
const MOCK_SLASH_COMMANDS: SlashCommand[] = [
  { id: '/tdd', label: '/tdd', description: 'Test-driven development workflow', icon: '🧪' },
  { id: '/code-review', label: '/code-review', description: 'Review current branch', icon: '🔍' },
  { id: '/plan', label: '/plan', description: 'Write an implementation plan', icon: '📋' },
  { id: '/debug', label: '/debug', description: 'Systematic debugging', icon: '🐛' },
  { id: '/explain', label: '/explain', description: 'Explain selected code', icon: '💬' },
]

export function getMockSlashCommands(): SlashCommand[] {
  return MOCK_SLASH_COMMANDS
}

// Simulate streaming a response in chunks every 30ms.
// Returns a cancel function.
export function simulateMarkdownStream(
  text: string,
  onChunk: (chunk: string) => void,
  onDone: () => void,
): () => void {
  const words = text.split(' ')
  let i = 0
  let cancelled = false

  function tick() {
    if (cancelled || i >= words.length) {
      if (!cancelled) onDone()
      return
    }
    onChunk((i === 0 ? '' : ' ') + words[i])
    i++
    setTimeout(tick, 30)
  }

  setTimeout(tick, 30)
  return () => { cancelled = true }
}
