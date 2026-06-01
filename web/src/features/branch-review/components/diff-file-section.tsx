import { memo, useEffect, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { CaretDown, CaretRight, FilePlus, FileText, FileX, Plus } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { ReviewThreadView } from './review-thread'
import type { GitDiff, GitDiffLine } from '@/features/git/types/git-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import { cn } from '@/utils/cn'

const LINE_THRESHOLD = 200

const EXT_LANG: Record<string, string> = {
  ts: 'typescript', tsx: 'tsx', js: 'javascript', jsx: 'jsx',
  go: 'go', py: 'python', rs: 'rust', json: 'json',
  css: 'css', html: 'html', md: 'markdown', sh: 'bash',
  yaml: 'yaml', yml: 'yaml', toml: 'toml', sql: 'sql',
}

type ShikiToken = { content: string; color: string }

function detectLang(filePath: string): string {
  const ext = filePath.split('.').pop() ?? ''
  return EXT_LANG[ext] ?? 'text'
}

function useShikiTokens(lines: GitDiffLine[], lang: string): Map<number, ShikiToken[]> {
  const [tokenMap, setTokenMap] = useState<Map<number, ShikiToken[]>>(new Map())

  useEffect(() => {
    if (lang === 'text') return
    const newLines = lines
      .filter(l => l.line_type === 'context' || l.line_type === 'added')
      .sort((a, b) => (a.new_line_number ?? 0) - (b.new_line_number ?? 0))

    const code = newLines.map(l => l.content).join('\n')
    let cancelled = false

    void (async () => {
      try {
        const { codeToTokens, bundledLanguages } = await import('shiki')
        if (!(lang in bundledLanguages)) return
        const { tokens } = await codeToTokens(code, {
          lang: lang as keyof typeof bundledLanguages,
          theme: 'github-dark-default',
        })
        if (!cancelled) {
          const map = new Map<number, ShikiToken[]>()
          newLines.forEach((line, idx) => {
            if (line.new_line_number != null) {
              map.set(line.new_line_number, (tokens[idx] ?? []).map(t => ({
                content: t.content,
                color: (t as { color?: string }).color ?? 'inherit',
              })))
            }
          })
          setTokenMap(map)
        }
      } catch { /* fall back to plain text */ }
    })()

    return () => { cancelled = true }
  }, [lines, lang])

  return tokenMap
}

export interface DiffFileSectionProps {
  diff: GitDiff
  threads: ReviewThread[]
  onAddThread: (filePath: string, lineNumber: number, side: 'left' | 'right') => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
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
  threads,
  filePath,
  tokenMap,
  onAddThread,
  onReply,
  onResolve,
}: {
  line: GitDiffLine
  threads: ReviewThread[]
  filePath: string
  tokenMap: Map<number, ShikiToken[]>
  onAddThread: (filePath: string, lineNumber: number, side: 'left' | 'right') => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
}) {
  // Every code line is commentable. Removed lines anchor to the left (old)
  // line number; added/context lines anchor to the right (new) one.
  const side: 'left' | 'right' = line.line_type === 'removed' ? 'left' : 'right'
  const anchorLine = side === 'left' ? line.old_line_number : line.new_line_number
  const canComment = line.line_type !== 'header' && anchorLine != null
  const lineThreads = threads.filter(t => t.side === side && t.lineNumber === anchorLine)
  const bg =
    line.line_type === 'added'
      ? 'bg-git-added/10'
      : line.line_type === 'removed'
        ? 'bg-git-deleted/10'
        : line.line_type === 'header'
          ? 'bg-muted/40 italic text-muted-foreground'
          : ''

  const tokens = line.new_line_number != null ? tokenMap.get(line.new_line_number) : undefined

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
          />
        </div>
      ))}
    </>
  )
}

type LinesProps = DiffFileSectionProps & { tokenMap: Map<number, ShikiToken[]> }

function FlatLines({ diff, threads, tokenMap, onAddThread, onReply, onResolve }: LinesProps) {
  return (
    <div>
      {diff.lines.map((line, i) => (
        <DiffLineRow
          key={i}
          line={line}
          threads={threads}
          filePath={diff.file_path}
          tokenMap={tokenMap}
          onAddThread={onAddThread}
          onReply={onReply}
          onResolve={onResolve}
        />
      ))}
    </div>
  )
}

function VirtualizedLines({ diff, threads, tokenMap, onAddThread, onReply, onResolve }: LinesProps) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: diff.lines.length,
    getScrollElement: () => parentRef.current,
    estimateSize: i => {
      const line = diff.lines[i]
      const lineThreads = threads.filter(t => t.lineNumber === line.new_line_number)
      return 22 + lineThreads.reduce((acc, t) => acc + 44 + t.messages.length * 56, 0)
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
              threads={threads}
              filePath={diff.file_path}
              tokenMap={tokenMap}
              onAddThread={onAddThread}
              onReply={onReply}
              onResolve={onResolve}
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
}: DiffFileSectionProps) {
  const [expanded, setExpanded] = useState(true)
  const fileName = diff.file_path.split('/').pop() ?? diff.file_path
  const isLarge = diff.lines.length > LINE_THRESHOLD
  const tokenMap = useShikiTokens(diff.lines, detectLang(diff.file_path))

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
        (isLarge ? (
          <VirtualizedLines
            diff={diff}
            threads={threads}
            tokenMap={tokenMap}
            onAddThread={onAddThread}
            onReply={onReply}
            onResolve={onResolve}
          />
        ) : (
          <FlatLines
            diff={diff}
            threads={threads}
            tokenMap={tokenMap}
            onAddThread={onAddThread}
            onReply={onReply}
            onResolve={onResolve}
          />
        ))}
    </div>
  )
})
