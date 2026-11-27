import { readLocal, removeLocal, writeLocal } from '@/lib/local-storage'

const draftKey = (chatId: string) => `crowbar:agent-draft:v1:${encodeURIComponent(chatId)}`

export function loadDraft(chatId: string): string {
  return readLocal(draftKey(chatId)) ?? ''
}

export function saveDraft(chatId: string, markdown: string): void {
  if (markdown.trim()) writeLocal(draftKey(chatId), markdown)
  else forgetDraft(chatId)
}

export function forgetDraft(chatId: string): void {
  removeLocal(draftKey(chatId))
}
