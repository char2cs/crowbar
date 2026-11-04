// A new thread is a pane on a client-minted id until its first prompt is sent;
// only then does the daemon get a chat. See components/layout/draft-thread.ts.
const DRAFT_PREFIX = 'draft-'

export function newDraftChatId(): string {
  return `${DRAFT_PREFIX}${crypto.randomUUID()}`
}

export function isDraftChatId(id: string | null | undefined): boolean {
  return !!id && id.startsWith(DRAFT_PREFIX)
}
