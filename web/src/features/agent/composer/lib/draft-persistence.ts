import { readLocal, removeLocal, writeLocal } from '@/lib/local-storage'

// Keyed by chat alone: a chat moved to another workspace keeps its draft.
const draftKey = (chatId: string) => `crowbar:agent-draft:v1:${encodeURIComponent(chatId)}`

/** The draft a chat's composer held when it last let go of it; '' when none. */
export function loadDraft(chatId: string): string {
  return readLocal(draftKey(chatId)) ?? ''
}

/** Keeps markdown as the chat's draft; an empty one is no draft. */
export function saveDraft(chatId: string, markdown: string): void {
  if (markdown.trim()) writeLocal(draftKey(chatId), markdown)
  else forgetDraft(chatId)
}

export function forgetDraft(chatId: string): void {
  removeLocal(draftKey(chatId))
}
