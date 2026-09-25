import { FileArchive, Image as ImageIcon } from '@phosphor-icons/react'
import { useState } from 'react'
import type { ReviewFileEntry } from '@/features/git/lib/review-placeholder'
import { cn } from '@/utils/cn'
import ImageDiffViewer from './git-diff-image'

/**
 * Binary files, kept out of the diff renderer entirely.
 *
 * They sit in their own block rather than inline in the CodeView because
 * CodeView renders exactly two things — a file and a diff — and a binary is
 * neither. Rows start collapsed so a review with fifty images neither decodes
 * fifty images nor blows the pane's height open.
 */
export function ReviewBinaryFiles({ entries }: { entries: readonly ReviewFileEntry[] }) {
  return (
    <section
      aria-label="Binary files"
      className="max-h-[50%] shrink-0 overflow-y-auto border-border border-b"
    >
      {entries.map((entry) => (
        <ReviewBinaryFile key={entry.path} entry={entry} />
      ))}
    </section>
  )
}

const BINARY_ROW_CLASS = 'flex w-full items-center gap-2 px-3 py-2 text-left ui-text-sm'

function ReviewBinaryFile({ entry }: { entry: ReviewFileEntry }) {
  const [open, setOpen] = useState(false)

  // A non-image binary has nothing to disclose, so it is a row and not a
  // control — a disabled button would announce itself as one that is broken.
  if (entry.kind !== 'image') {
    return (
      <div className="border-border/70 border-b last:border-b-0">
        <div className={BINARY_ROW_CLASS}>
          <BinaryRowLabel path={entry.path} label="Binary file" icon="binary" />
        </div>
      </div>
    )
  }

  return (
    <div className="border-border/70 border-b last:border-b-0">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        aria-expanded={open}
        className={cn(BINARY_ROW_CLASS, 'hover:bg-muted/30')}
      >
        <BinaryRowLabel path={entry.path} label="Image" icon="image" />
      </button>
      {open ? (
        <div className="h-96 border-border/70 border-t">
          <ImageDiffViewer diff={entry.file} fileName={entry.path} />
        </div>
      ) : null}
    </div>
  )
}

function BinaryRowLabel({
  path,
  label,
  icon,
}: {
  path: string
  label: string
  icon: 'image' | 'binary'
}) {
  return (
    <>
      {icon === 'image' ? (
        <ImageIcon className="shrink-0 text-muted-foreground" />
      ) : (
        <FileArchive className="shrink-0 text-muted-foreground" />
      )}
      <span className="min-w-0 flex-1 truncate editor-font">{path}</span>
      <span className="shrink-0 text-muted-foreground ui-text-xs">{label}</span>
    </>
  )
}
