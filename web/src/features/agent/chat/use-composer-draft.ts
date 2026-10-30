import { useCallback, useRef, useState } from 'react'
import {
  addPromptStash,
  createPromptStash,
  loadPromptStashes,
  savePromptStashes,
  type PromptStash,
} from '@/features/agent/composer/lib/prompt-stash-persistence'
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

/**
 * The composer's text, the slash picker that filters on it, its stashes and its
 * one error line. The box is
 * uncontrolled, so text pushed in arrives by remount from `seed`, which carries
 * its own text: an async onChange can land after a send bumped it.
 */
export function useComposerDraft({ wsId, chatId, providerId, catalogActive, history }: DraftDeps) {
  const [draft, setDraft] = useState('')
  const slash = useSlashCatalog({ wsId, chatId, providerId, active: catalogActive, draft })
  const [seed, setSeed] = useState({ text: '', n: 0 })
  const [stashes, setStashes] = useState(() => loadPromptStashes(wsId, chatId))
  const [stashPickerOpen, setStashPickerOpen] = useState(false)
  const [error, setError] = useState('')

  // What the box holds now, for text that arrives after the render that asked for it.
  const latest = useRef('')
  const seedDraft = useCallback((value: string) => {
    latest.current = value
    setDraft(value)
    setSeed((previous) => ({ text: value, n: previous.n + 1 }))
  }, [])

  /** Puts text back without losing what was typed since: it goes after it. */
  const putBack = useCallback(
    (text: string) => {
      const typed = latest.current.trimEnd()
      seedDraft(typed ? `${typed}\n\n${text}` : text)
    },
    [seedDraft],
  )

  const updateDraft = (value: string) => {
    latest.current = value
    setDraft(value)
    setError('')
    setStashPickerOpen(false)
    slash.noteDraft(value)
    history.reset()
  }

  const restoreStash = (item: PromptStash) => {
    // The picker can outlive an edit; never replace words typed after it opened.
    if (draft.trim()) {
      setStashPickerOpen(false)
      setError('Stash or clear the current draft before restoring another one.')
      return
    }
    const remaining = stashes.filter((candidate) => candidate.id !== item.id)
    if (!savePromptStashes(wsId, chatId, remaining)) {
      setError('Could not update saved prompt stashes. The current draft was not changed.')
      return
    }
    setStashes(remaining)
    setStashPickerOpen(false)
    seedDraft(item.markdown)
    setError('')
    slash.noteDraft(item.markdown)
    history.reset()
  }

  const stashOrRestore = (markdown: string) => {
    if (markdown.trim()) {
      const item = createPromptStash(markdown)
      if (!item) {
        setError('This draft is too large to stash.')
        return
      }
      const next = addPromptStash(stashes, item)
      if (!savePromptStashes(wsId, chatId, next)) {
        setError('Could not save this prompt stash. The current draft was not changed.')
        return
      }
      setStashes(next)
      setStashPickerOpen(false)
      seedDraft('')
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
    draft,
    slash,
    seed,
    seedDraft,
    putBack,
    updateDraft,
    stashes,
    stashPickerOpen,
    closeStashPicker: () => setStashPickerOpen(false),
    restoreStash,
    stashOrRestore,
    error,
    setError,
  }
}
