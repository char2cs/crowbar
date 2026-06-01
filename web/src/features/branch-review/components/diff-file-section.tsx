import { memo, useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { CaretDown, CaretRight, FilePlus, FileText, FileX, Plus } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { ReviewThreadView } from './review-thread'
import {
  type ShikiToken,
  detectLang,
  ensureHighlighter,
  tokenizeLine,
} from '../lib/diff-highlighter'
import type { Highlighter } from 'shiki'
import type { GitDiff, GitDiffLine } from '@/features/git/types/git-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import { cn } from '@/utils/cn'

const LINE_THRESHOLD = 200
const EMPTY_THREADS: ReviewThread[] = []

type ThreadsByLine = Map<string, ReviewThread[]>

function threadKey(side: 'left' | 'right', line: number) {
  return `${side}:${line}`
}

/** Load the grammar for this file's language once; re-render when it's ready. */
function useDiffHighlighter(lang: string): Highlighter | null {
  const [hl, setHl] = useState<Highlighter | null>(null)
  useEffect(() => {
    let cancelled = false
    setHl(null)
    void ensureHighlighter(lang).then(h => { if (!cancelled) setHl(h) })
    return () => { cancelled = true }
  }, [lang])
  return hl
}

export interface DiffFileSectionProps {
  diff: GitDiff
  threads: ReviewThread[]
  onAddThread: (filePath: string, lineNumber: number, side: 'left' | 'right') => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
  onDelete: (threadId: string) => void
}

function statusIcon(diff: GitDiff) {
  if (diff.is_new) return <FilePlus size={13} className="shrink-0 text-git-added" />
  if (diff.is_deleted) return <FileX size={13} className="shrink-0 text-git-deleted" />
  return <FileText size={13} className="shrink-0 text-git-modified" />
}

function statusColor(diff: GitDiff) {
  if (diff.is_new) return 'text-git-added'
  if (diff.is_deleted) return 'text-git-deleted'
  return 'text-git-modified'
}

function DiffLineRow({
  line,
  threadsByLine,
  filePath,
  highlighter,
  lang,
  onAddThread,
  onReply,
  onResolve,
  onDelete,
}: {
  line: GitDiffLine
  threadsByLine: ThreadsByLine
  filePath: string
  highlighter: Highlighter | null
  lang: string
  onAddThread: (filePath: string, lineNumber: number, side: 'left' | 'right') => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
  onDelete: (threadId: string) => void
}) {
  // Every code line is commentable. Removed lines anchor to the left (old)
  // line number; added/context lines anchor to the right (new) one.
  const side: 'left' | 'right' = line.line_type === 'removed' ? 'left' : 'right'
  const anchorLine = side === 'left' ? line.old_line_number : line.new_line_number
  const canComment = line.line_type !== 'header' && anchorLine != null
  const lineThreads =
    canComment ? threadsByLine.get(threadKey(side, anchorLine!)) ?? EMPTY_THREADS : EMPTY_THREADS
  const bg =
    line.line_type === 'added'
      ? 'bg-git-added/10'
      : line.line_type === 'removed'
        ? 'bg-git-deleted/10'
        : line.line_type === 'header'
          ? 'bg-muted/40 italic text-muted-foreground'
          : ''

  // Tokenise only this (rendered) line, synchronously, once the grammar is
  // loaded. Result is memoised by content in the shared cache.
  let tokens: ShikiToken[] | null = null
  if (highlighter && line.line_type !== 'header') {
    tokens = tokenizeLine(highlighter, lang, line.content)
  }

  return (
    <>
      <div className={cn('group flex min-h-[22px] items-start font-mono text-xs', bg)}>
        <div className="relative flex w-20 shrink-0 select-none items-center justify-end gap-1 border-r border-border/30 px-2 text-[10px] text-muted-foreground/40">
          {line.old_line_number != null && (
            <span className="w-6 text-right">{line.old_line_number}</span>
          )}
          {line.new_line_number != null && (
            <span className="w-6 text-right">{line.new_line_number}</span>
          )}
          {canComment && (
            <Button
              size="icon-xs"
              aria-label="Add comment"
              onClick={() => onAddThread(filePath, anchorLine!, side)}
              className="absolute left-0.5 top-1/2 hidden -translate-y-1/2 group-hover:inline-flex"
            >
              <Plus weight="bold" />
            </Button>
          )}
        </div>
        <div className="flex-1 overflow-hidden break-all whitespace-pre-wrap px-2 py-[2px] leading-[18px]">
          <span
            className={cn(
              'mr-1',
              line.line_type === 'added'
                ? 'text-git-added'
                : line.line_type === 'removed'
                  ? 'text-git-deleted'
                  : 'text-transparent',
            )}
          >
            {line.line_type === 'added' ? '+' : line.line_type === 'removed' ? '-' : ' '}
          </span>
          {tokens?.length ? (
            tokens.map((tok, i) => (
              <span key={i} style={{ color: tok.color }}>{tok.content}</span>
            ))
          ) : (
            line.content
          )}
        </div>
      </div>
      {lineThreads.map(thread => (
        <div key={thread.id} className="px-4">
          <ReviewThreadView
            thread={thread}
            onReply={body => onReply(thread.id, body)}
            onResolve={() => onResolve(thread.id)}
            onDelete={() => onDelete(thread.id)}
          />
        </div>
      ))}
    </>
  )
}

type LinesProps = {
  diff: GitDiff
  threadsByLine: ThreadsByLine
  highlighter: Highlighter | null
  lang: string
  onAddThread: (filePath: string, lineNumber: number, side: 'left' | 'right') => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
  onDelete: (threadId: string) => void
}

function FlatLines({ diff, threadsByLine, highlighter, lang, onAddThread, onReply, onResolve, onDelete }: LinesProps) {
  return (
    <div>
      {diff.lines.map((line, i) => (
        <DiffLineRow
          key={i}
          line={line}
          threadsByLine={threadsByLine}
          filePath={diff.file_path}
          highlighter={highlighter}
          lang={lang}
          onAddThread={onAddThread}
          onReply={onReply}
          onResolve={onResolve}
          onDelete={onDelete}
        />
      ))}
    </div>
  )
}

function lineAnchor(line: GitDiffLine): { side: 'left' | 'right'; n: number | null } {
  const side: 'left' | 'right' = line.line_type === 'removed' ? 'left' : 'right'
  return { side, n: side === 'left' ? line.old_line_number ?? null : line.new_line_number ?? null }
}

function VirtualizedLines({ diff, threadsByLine, highlighter, lang, onAddThread, onReply, onResolve, onDelete }: LinesProps) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: diff.lines.length,
    getScrollElement: () => parentRef.current,
    estimateSize: i => {
      const line = diff.lines[i]
      const { side, n } = lineAnchor(line)
      const lineThreads = n != null ? threadsByLine.get(threadKey(side, n)) : undefined
      return 22 + (lineThreads?.reduce((acc, t) => acc + 44 + t.messages.length * 56, 0) ?? 0)
    },
    measureElement: el => el.getBoundingClientRect().height,
    overscan: 10,
  })

  return (
    <div ref={parentRef} style={{ maxHeight: 600, overflowY: 'auto' }}>
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map(vItem => (
          <div
            key={vItem.key}
            data-index={vItem.index}
            ref={virtualizer.measureElement}
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              width: '100%',
              transform: `translateY(${vItem.start}px)`,
            }}
          >
            <DiffLineRow
              line={diff.lines[vItem.index]}
              threadsByLine={threadsByLine}
              filePath={diff.file_path}
              highlighter={highlighter}
              lang={lang}
              onAddThread={onAddThread}
              onReply={onReply}
              onResolve={onResolve}
              onDelete={onDelete}
            />
          </div>
        ))}
      </div>
    </div>
  )
}

export const DiffFileSection = memo(function DiffFileSection({
  diff,
  threads,
  onAddThread,
  onReply,
  onResolve,
  onDelete,
}: DiffFileSectionProps) {
  const [expanded, setExpanded] = useState(true)
  const fileName = diff.file_path.split('/').pop() ?? diff.file_path
  const isLarge = diff.lines.length > LINE_THRESHOLD
  const lang = detectLang(diff.file_path)
  const highlighter = useDiffHighlighter(lang)

  // Pre-group threads by anchor (side + line) so each rendered row is an O(1)
  // lookup instead of an O(threads) filter — the old code filtered per line,
  // including once per line inside the virtualizer's estimateSize.
  const threadsByLine = useMemo<ThreadsByLine>(() => {
    const map: ThreadsByLine = new Map()
    for (const t of threads) {
      const key = threadKey(t.side, t.lineNumber)
      const existing = map.get(key)
      if (existing) existing.push(t)
      else map.set(key, [t])
    }
    return map
  }, [threads])

  const linesProps: LinesProps = {
    diff,
    threadsByLine,
    highlighter,
    lang,
    onAddThread,
    onReply,
    onResolve,
    onDelete,
  }

  return (
    <div className="border-b border-border last:border-b-0">
      <div
        className="flex cursor-pointer items-center gap-2 bg-muted/30 px-3 py-1.5 transition-colors hover:bg-muted/50"
        onClick={() => setExpanded(e => !e)}
      >
        {expanded ? (
          <CaretDown size={12} className="text-muted-foreground" />
        ) : (
          <CaretRight size={12} className="text-muted-foreground" />
        )}
        {statusIcon(diff)}
        <span className={cn('flex-1 truncate text-xs font-medium', statusColor(diff))}>
          {fileName}
        </span>
        <span className="text-[10px] text-muted-foreground">{diff.file_path}</span>
        <div className="ml-2 flex items-center gap-1.5 text-[10px]">
          {(diff.additions ?? 0) > 0 && (
            <span className="text-git-added">+{diff.additions}</span>
          )}
          {(diff.deletions ?? 0) > 0 && (
            <span className="text-git-deleted">-{diff.deletions}</span>
          )}
        </div>
      </div>
      {expanded &&
        (isLarge ? <VirtualizedLines {...linesProps} /> : <FlatLines {...linesProps} />)}
    </div>
  )
})
