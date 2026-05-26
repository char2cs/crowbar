import type { ChatMessage } from '@/lib/types'

const MOCK_MESSAGES: Record<string, ChatMessage[]> = {
  'ws3:brainstorming': [
    {
      id: 'm1', role: 'user',
      content: 'How should we handle auth across crowbar, quiver.core and quiver.desktop?',
      authorName: 'Mateo', authorInitials: 'MU', timestamp: '2h ago',
    },
    {
      id: 'm2', role: 'assistant',
      content: `Given all three share a user identity, a **shared auth service** makes the most sense. Here's my recommendation:

## Architecture

- **Token issuance** — lightweight Go service, issues JWTs on login
- **Refresh** — sliding 7-day window, short-lived access tokens
- **Verification** — each app verifies locally, no round-trips

## Implementation

\`\`\`go
func IssueToken(userID string) (string, error) {
  claims := jwt.MapClaims{
    "sub": userID,
    "exp": time.Now().Add(15 * time.Minute).Unix(),
  }
  return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
    SignedString([]byte(os.Getenv("JWT_SECRET")))
}
\`\`\`

Each app calls \`VerifyToken(token)\` locally — _no_ shared database lookups needed.`,
      authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
      timestamp: '2h ago · 18.3s', toolCalls: 4, durationSec: 18.3,
    },
    {
      id: 'm3', role: 'user', content: "Makes sense. Let's go with that.",
      authorName: 'Mateo', authorInitials: 'MU', timestamp: '2h ago',
    },
  ],
  'ws2:implementation': [
    {
      id: 'i1', role: 'assistant',
      content: 'Starting implementation. Creating tasks for the API backend...',
      authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
      timestamp: '1d ago',
    },
  ],
}

export function getMockConversation(wsId: string, step: string): ChatMessage[] {
  return MOCK_MESSAGES[`${wsId}:${step}`] ?? []
}
