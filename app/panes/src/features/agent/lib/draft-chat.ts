const DRAFT_PREFIX = 'draft-'

export function newDraftChatId(): string {
  return `${DRAFT_PREFIX}${crypto.randomUUID()}`
}

export function isDraftChatId(id: string | null | undefined): boolean {
  return !!id && id.startsWith(DRAFT_PREFIX)
}
