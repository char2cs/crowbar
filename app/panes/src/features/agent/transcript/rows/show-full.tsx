import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { showFull } from '@/features/agent/stores/chat-feed'
import { report } from '@/lib/errors/report'

export function ShowFull({
  chatId,
  id,
  fullSeq,
  what,
}: {
  chatId: string
  id: string
  fullSeq: number | undefined
  what: string
}) {
  const [loading, setLoading] = useState(false)
  if (fullSeq === undefined) return null
  const load = () => {
    setLoading(true)
    showFull(chatId, id)
      .catch((err: unknown) => report(err, { action: `Load the full ${what}` }))
      .finally(() => setLoading(false))
  }
  return (
    <p className="row-preview-note">
      <span>Preview only.</span>
      <Button variant="link" size="xs" loading={loading} onClick={load}>
        Show full {what}
      </Button>
    </p>
  )
}
