import { useCallback, useRef, useState, useSyncExternalStore } from 'react'
import {
  addPromptStash,
  createPromptStash,
  loadPromptStashes,
  promptStashesSnapshot,
  savePromptStashes,
  subscribePromptStashes,
  type PromptStash,
} from '@/features/agent/composer/lib/prompt-stash-persistence'
import type {
  ChatMarkdownEditorHandle,
  ComposerShape,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { useSlashCatalog } from '@/features/agent/hooks/use-slash-catalog'
import { useFileMention } from '@/features/agent/hooks/use-file-mention'

interface DraftDeps {
  wsId: string
  chatId: string
  providerId: string
  catalogActive: boolean
  history: { reset: () => void }
}

export function useComposerDraft({ wsId, chatId, providerId, catalogActive, history }: DraftDeps) {
  const editorRef = useRef<ChatMarkdownEditorHandle>(null)
  const slash = useSlashCatalog({ wsId, chatId, providerId, active: catalogActive })
  const mention = useFileMention(wsId)
  const stashes = useSyncExternalStore(subscribePromptStashes, () =>
    promptStashesSnapshot(wsId, chatId),
  )
  const [stashPickerOpen, setStashPickerOpen] = useState(false)
  const [error, setError] = useState('')

  const read = useCallback(() => editorRef.current?.getMarkdown() ?? '', [])
  const load = useCallback((text: string) => editorRef.current?.setMarkdown(text), [])
  const clear = useCallback(() => editorRef.current?.clear(), [])
  const completeMention = useCallback(
    (text: string) => editorRef.current?.completeMention(text),
    [],
  )
  const completeCommand = useCallback(
    (text: string) => editorRef.current?.completeCommand(text),
    [],
  )

  const putBack = useCallback((text: string) => {
    if (!editorRef.current) return false
    editorRef.current.appendMarkdown(text)
    return true
  }, [])

  const noteShape = (shape: ComposerShape, edited: boolean) => {
    if (edited) history.reset()
    if (error) setError('')
    if (stashPickerOpen) setStashPickerOpen(false)
    slash.noteQuery(shape.slashQuery)
    mention.noteQuery(shape.mentionQuery)
  }

  const restoreStash = (item: PromptStash) => {
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
    mention,
    completeMention,
    completeCommand,
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
