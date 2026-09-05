'use client'

import { useState } from 'react'
import { FileText } from '@phosphor-icons/react'
import { AppDialog } from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'

interface TextAttachmentPillProps {
  text: string
}

/** Empty text is 0 lines, not the 1 an unconditional `split('\n')` would give. */
function countLines(text: string): number {
  return text.length === 0 ? 0 : text.split('\n').length
}

/** Pasted-text attachment: a pill showing the line count, and a read-only
 *  modal showing the raw text verbatim — no markdown rendering (per the
 *  design spec, "no markdown for now"). */
export function TextAttachmentPill({ text }: TextAttachmentPillProps) {
  const [open, setOpen] = useState(false)
  const lineCount = countLines(text)

  return (
    <>
      <Badge
        variant="outline"
        onClick={() => setOpen(true)}
        render={<button type="button" />}
        className="h-auto min-w-0 cursor-pointer gap-1.5 rounded-full px-2.5 py-1 font-normal text-muted-foreground text-xs hover:bg-accent/50"
      >
        <FileText size={14} />
        Pasted text · {lineCount} {lineCount === 1 ? 'line' : 'lines'}
      </Badge>
      {open && (
        <AppDialog
          title="Pasted text"
          icon={FileText}
          size="lg"
          onClose={() => setOpen(false)}
          classNames={{ content: 'overflow-auto p-4' }}
        >
          <pre
            data-testid="text-attachment-raw"
            className="whitespace-pre-wrap break-words font-mono text-xs"
          >
            {text}
          </pre>
        </AppDialog>
      )}
    </>
  )
}
