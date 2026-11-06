import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from 'react'
import {
  addPromptStash,
  createPromptStash,
  loadPromptStashes,
  promptStashesSnapshot,
  savePromptStashes,
  subscribePromptStashes,
  type PromptStash,
} from '@/features/agent/composer/lib/prompt-stash-persistence'
import { forgetDraft, loadDraft, saveDraft } from '@/features/agent/composer/lib/draft-persistence'
import type {
  ChatMarkdownEditorHandle,
  ComposerShape,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { useSlashCatalog } from '@/features/agent/hooks/use-slash-catalog'

interface DraftDeps {
  wsId: string
  chatId: string
  providerId: string
  /** A parked tab probes its slash catalogue the moment it is shown, not before. */
  catalogActive: boolean
  /** History recall restarts from the newest prompt after any edit. */
  history: { reset: () => void }
}

/** The composer's text, the slash picker that filters on it, its stashes and its
 *  one error line. The text lives in the editor only: this reads it on demand and
 *  loads text into it through the handle. */
export function useComposerDraft({ wsId, chatId, providerId, catalogActive, history }: DraftDeps) {
  const editorRef = useRef<ChatMarkdownEditorHandle>(null)
  const slash = useSlashCatalog({ wsId, chatId, providerId, active: catalogActive })
  // Read from storage, never copied: the outbox stashes a failed prompt while this is open.
  const stashes = useSyncExternalStore(subscribePromptStashes, () =>
    promptStashesSnapshot(wsId, chatId),
  )
  const [stashPickerOpen, setStashPickerOpen] = useState(false)
  const [error, setError] = useState('')

  const read = useCallback(() => editorRef.current?.getMarkdown() ?? '', [])
  const load = useCallback((text: string) => editorRef.current?.setMarkdown(text), [])
  const clear = useCallback(() => {
    forgetDraft(chatId)
    editorRef.current?.clear()
  }, [chatId])

  // The editor owns the draft while mounted; storage keeps it across a reload or
  // a remount, written as the page hides or the view goes (plan: persisted drafts).
  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return
    const saved = loadDraft(chatId)
    if (saved && !editor.getMarkdown().trim()) editor.restoreMarkdown(saved)
    const save = () => saveDraft(chatId, editor.getMarkdown())
    window.addEventListener('pagehide', save)
    return () => {
      window.removeEventListener('pagehide', save)
      save()
    }
  }, [chatId])

  /** Puts text back after what was typed since; false when there is no box to take it. */
  const putBack = useCallback((text: string) => {
    if (!editorRef.current) return false
    editorRef.current.appendMarkdown(text)
    return true
  }, [])

  // Fires per edit: only what changed sets state, so plain typing renders nothing.
  // Any edit, keyed or not (a drop, a paste menu), abandons a walk through history.
  const noteShape = (shape: ComposerShape, edited: boolean) => {
    if (edited) history.reset()
    if (error) setError('')
    if (stashPickerOpen) setStashPickerOpen(false)
    slash.noteQuery(shape.slashQuery)
  }

  const restoreStash = (item: PromptStash) => {
    // The picker can outlive an edit; never replace words typed after it opened.
    if (read().trim()) {
      setStashPickerOpen(false)
      setError('Stash or clear the current draft before restoring another one.')
      return
    }
    const remaining = loadPromptStashes(wsId, chatId).filter(
      (candidate) => candidate.id !== item.id,
    )
    if (!savePromptStashes(wsId, chatId, remaining)) {
      setError('Could not update saved prompt stashes. The current draft was not changed.')
      return
    }
    setStashPickerOpen(false)
    load(item.markdown)
    setError('')
    history.reset()
  }

  const stashOrRestore = (markdown: string) => {
    if (markdown.trim()) {
      const item = createPromptStash(markdown)
      if (!item) {
        setError('This draft is too large to stash.')
        return
      }
      const next = addPromptStash(loadPromptStashes(wsId, chatId), item)
      if (!savePromptStashes(wsId, chatId, next)) {
        setError('Could not save this prompt stash. The current draft was not changed.')
        return
      }
      setStashPickerOpen(false)
      clear()
      setError('')
      slash.reset()
      history.reset()
      return
    }
    if (stashes.length === 0) return
    if (stashes.length === 1) {
      restoreStash(stashes[0])
      return
    }
    setError('')
    setStashPickerOpen(true)
  }

  return {
    editorRef,
    read,
    load,
    clear,
    slash,
    putBack,
    noteShape,
    stashes,
    stashPickerOpen,
    closeStashPicker: () => setStashPickerOpen(false),
    restoreStash,
    stashOrRestore,
    error,
    setError,
  }
}
