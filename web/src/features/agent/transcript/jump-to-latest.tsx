import { ArrowDownIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'

/** Back to the newest rows, for a reader up the page while the chat goes on below. */
export function JumpToLatest({ onJump }: { onJump: () => void }) {
  return (
    <Button size="sm" variant="secondary" className="jump-latest" onClick={onJump}>
      <ArrowDownIcon /> Jump to latest
    </Button>
  )
}
