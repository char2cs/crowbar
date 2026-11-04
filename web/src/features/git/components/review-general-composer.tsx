import { useId, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import type { NewThreadInput } from '@/features/git/hooks/use-review-thread-actions'

/** Anchored to no file or line: it belongs to the branch as a whole. */
const GENERAL_THREAD: Omit<NewThreadInput, 'body'> = {
  filePath: '',
  line: 0,
  startLine: 0,
  endLine: 0,
  side: 'new',
}

interface ReviewGeneralComposerProps {
  postThread: (input: NewThreadInput) => Promise<boolean>
  error: string | null
}

export function ReviewGeneralComposer({ postThread, error }: ReviewGeneralComposerProps) {
  const [body, setBody] = useState('')
  const [posting, setPosting] = useState(false)
  const inputId = useId()
  const trimmed = body.trim()

  const submit = async () => {
    if (!trimmed || posting) return
    setPosting(true)
    try {
      const posted = await postThread({ ...GENERAL_THREAD, body: trimmed })
      // A failed post keeps the text: the user cannot see why it failed.
      if (posted) setBody('')
    } finally {
      setPosting(false)
    }
  }

  return (
    <section
      aria-label="Leave a comment"
      className="flex flex-col gap-2.5 rounded-xl border border-border bg-card p-4"
    >
      <label htmlFor={inputId} className="ui-text-base font-semibold">
        Leave a comment
      </label>
      <Textarea
        id={inputId}
        rows={3}
        placeholder="Agents working on this branch pick this up on their own."
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault()
            void submit()
          }
        }}
      />
      <div className="flex items-center justify-end gap-2">
        {error && <span className="ui-text-xs mr-auto text-destructive">{error}</span>}
        <Button size="sm" disabled={!trimmed || posting} onClick={() => void submit()}>
          Comment
        </Button>
      </div>
    </section>
  )
}
