'use client'

import { useState } from 'react'
import { FileText } from '@phosphor-icons/react'
import { AppDialog } from '@/components/ui/dialog'
import { ATTACHMENT_BOX_CLASS } from './attachment-box'

interface TextAttachmentPillProps {
  text: string
}

/** Empty text is 0 lines, not the 1 an unconditional `split('\n')` would give. */
function countLines(text: string): number {
  return text.length === 0 ? 0 : text.split('\n').length
}

/** Pasted-text attachment: a boxed card (same footprint every attachment
 *  kind now shares — see attachment-box.ts) showing the line count, and a
 *  read-only modal showing the raw text verbatim — no markdown rendering
 *  (per the design spec, "no markdown for now"). Was a thin, single-line
 *  pill; reported live as needing to look like an actual attachment (Claude's
 *  own image-attachment card was the reference point), not a tag. */
export function TextAttachmentPill({ text }: TextAttachmentPillProps) {
  const [open, setOpen] = useState(false)
  const lineCount = countLines(text)

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={ATTACHMENT_BOX_CLASS}>
        <FileText size={28} className="shrink-0 text-muted-foreground" />
        <span className="line-clamp-2 w-full break-words px-1 font-medium text-foreground text-xs">
          Pasted text
        </span>
        <span className="text-[11px] text-muted-foreground">
          {lineCount} {lineCount === 1 ? 'line' : 'lines'}
        </span>
      </button>
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
