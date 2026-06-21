import { memo, useMemo, useState } from 'react'
import { Plus } from '@phosphor-icons/react'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'
import { cn } from '@/utils/cn'
import type { AddCommentAnchor, DiffLineProps } from '../../types/git-diff-types'
import { getDiffLineVisualState, getDiffLineVisualType } from '../../utils/git-diff-helpers'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { ReviewThreadItem } from '../review-thread-item'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import { openThread, replyToThread, setThreadResolved } from '@/features/git/api/review-api'

export const getLineBackground = (type: string) => {
  return getDiffLineVisualState(getDiffLineVisualType(type as DiffLineProps['line']['line_type']))
    .lineBackground
}

export const getGutterBackground = (type: string) => {
  return getDiffLineVisualState(getDiffLineVisualType(type as DiffLineProps['line']['line_type']))
    .gutterBackground
}

export const getContentColor = (type: string) => {
  return getDiffLineVisualState(getDiffLineVisualType(type as DiffLineProps['line']['line_type']))
    .contentColor
}

const renderWhitespace = (content: string, showWhitespace: boolean) => {
  if (!showWhitespace) return content

  return content.split('').map((char, i) => {
    if (char === ' ') {
      return (
        <span key={i} className="text-muted-foreground opacity-30">
          ·
        </span>
      )
    }
    if (char === '\t') {
      return (
        <span key={i} className="text-muted-foreground opacity-30">
          →{'   '}
        </span>
      )
    }
    return char
  })
}

const renderHighlightedContent = (
  content: string,
  tokens: HighlightToken[] | undefined,
  showWhitespace: boolean,
) => {
  if (!tokens || tokens.length === 0) {
    return <span>{renderWhitespace(content, showWhitespace)}</span>
  }

  const result: React.ReactNode[] = []
  let lastEnd = 0

  for (const [tokenIndex, token] of tokens.entries()) {
    const start = token.startPosition.column
    const end = token.endPosition.column

    if (start > lastEnd) {
      const text = content.slice(lastEnd, start)
      result.push(
        <span key={`plain-${lastEnd}-${tokenIndex}`}>
          {renderWhitespace(text, showWhitespace)}
        </span>,
      )
    }

    const tokenText = content.slice(start, end)
    const scopeClass = token.type

    result.push(
      <span key={`token-${start}-${end}-${tokenIndex}`} className={scopeClass}>
        {renderWhitespace(tokenText, showWhitespace)}
      </span>,
    )

    lastEnd = end
  }

  if (lastEnd < content.length) {
    const text = content.slice(lastEnd)
    result.push(<span key={`plain-tail-${lastEnd}`}>{renderWhitespace(text, showWhitespace)}</span>)
  }

  return <>{result}</>
}

export function renderDiffLineContent(
  content: string,
  tokens: HighlightToken[] | undefined,
  showWhitespace: boolean,
) {
  return renderHighlightedContent(content, tokens, showWhitespace)
}

export function getSplitLineMeta(line: DiffLineProps['line'], splitSide: 'left' | 'right') {
  const isLeft = splitSide === 'left'
  const isVisible = isLeft ? line.line_type !== 'added' : line.line_type !== 'removed'
  const gutterNumber = isLeft ? line.old_line_number : line.new_line_number
  const diffType = isLeft
    ? line.line_type === 'removed'
      ? 'removed'
      : 'context'
    : line.line_type === 'added'
      ? 'added'
      : 'context'

  return {
    isVisible,
    gutterNumber,
    diffType,
  }
}

/** Derive the canonical anchor side for a diff line in unified view. */
function getUnifiedSide(lineType: DiffLineProps['line']['line_type']): 'old' | 'new' {
  return lineType === 'removed' ? 'old' : 'new'
}

/** Derive the canonical anchor line number for a diff line in unified view. */
function getUnifiedLineNumber(line: DiffLineProps['line']): number | undefined {
  if (line.line_type === 'removed') return line.old_line_number
  return line.new_line_number
}

/** Find threads anchored to this line in unified view. */
function getThreadsForLine(
  threads: ReviewThread[],
  filePath: string,
  line: DiffLineProps['line'],
): ReviewThread[] {
  const side = getUnifiedSide(line.line_type)
  const lineNumber = getUnifiedLineNumber(line)
  if (lineNumber === undefined) return []
  return threads.filter(
    (t) => t.filePath === filePath && t.side === side && t.lineNumber === lineNumber,
  )
}

/** Thread rows rendered below a diff line. */
function ThreadRows({
  threads,
  wsId,
}: {
  threads: ReviewThread[]
  wsId: string
}) {
  const handleReply = async (threadId: string, body: string) => {
    await replyToThread(wsId, threadId, { body })
  }
  const handleResolve = async (threadId: string) => {
    await setThreadResolved(wsId, threadId, true)
  }
  const handleReopen = async (threadId: string) => {
    await setThreadResolved(wsId, threadId, false)
  }

  if (threads.length === 0) return null

  return (
    <div className="border-border border-b bg-muted/10 px-2 py-1">
      {threads.map((thread) => (
        <ReviewThreadItem
          key={thread.id}
          thread={thread}
          wsId={wsId}
          onReply={handleReply}
          onResolve={handleResolve}
          onReopen={handleReopen}
        />
      ))}
    </div>
  )
}

/** Inline comment composer row. */
function ComposerRow({
  anchor,
  onSubmit,
  onCancel,
}: {
  anchor: AddCommentAnchor
  wsId: string
  onSubmit: (body: string) => Promise<void>
  onCancel: () => void
}) {
  const lineLabel = `line ${anchor.line}`

  return (
    <div className="border-border border-b bg-muted/10 px-2 py-2">
      <CommentComposer
        title={`Comment on ${lineLabel}`}
        submitLabel="Comment"
        onSubmit={onSubmit}
        onCancel={onCancel}
      />
    </div>
  )
}

/** Gutter "+" button — visible on hover of the parent row. */
function AddCommentButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      aria-label="Add comment"
      data-testid="add-comment-btn"
      className={cn(
        'absolute right-0.5 top-1/2 -translate-y-1/2',
        'flex size-4 items-center justify-center rounded',
        'text-info opacity-0 transition-opacity group-hover/diffrow:opacity-100',
        'hover:bg-info/10 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
      )}
      onClick={(e) => {
        e.stopPropagation()
        onClick()
      }}
    >
      <Plus className="size-3" weight="bold" />
    </button>
  )
}

const DiffLine = memo(
  ({
    line,
    viewMode,
    splitSide,
    wordWrap,
    showWhitespace,
    tokens,
    fontSize,
    lineHeight,
    tabSize,
    filePath,
    wsId,
    threads,
    onAddComment,
  }: DiffLineProps) => {
    const rowStyle = { minHeight: `${lineHeight}px` }
    const gutterStyle = { fontSize: `${fontSize}px`, lineHeight: `${lineHeight}px` }
    const contentStyle = {
      fontSize: `${fontSize}px`,
      lineHeight: `${lineHeight}px`,
      tabSize,
      whiteSpace: wordWrap ? ('pre-wrap' as const) : ('pre' as const),
      overflowWrap: wordWrap ? ('anywhere' as const) : ('normal' as const),
      wordBreak: wordWrap ? ('break-word' as const) : ('normal' as const),
    }

    const [composerAnchor, setComposerAnchor] = useState<AddCommentAnchor | null>(null)

    const lineContent = useMemo(() => {
      return renderHighlightedContent(line.content, tokens, showWhitespace)
    }, [line.content, tokens, showWhitespace])

    // Only show "+" for lines that have a real line number (not headers)
    const canComment =
      !!onAddComment && !!filePath && !!wsId && line.line_type !== 'header'

    const handleAddComment = () => {
      if (!filePath) return
      const side = getUnifiedSide(line.line_type)
      const lineNumber = getUnifiedLineNumber(line)
      if (lineNumber === undefined) return
      const anchor: AddCommentAnchor = { filePath, side, line: lineNumber }
      if (onAddComment) onAddComment(anchor)
      setComposerAnchor(anchor)
    }

    const handleComposerSubmit = async (body: string) => {
      if (!wsId || !composerAnchor) return
      await openThread(wsId, {
        filePath: composerAnchor.filePath,
        line: composerAnchor.line,
        startLine: composerAnchor.line,
        endLine: composerAnchor.line,
        side: composerAnchor.side,
        body,
      })
      setComposerAnchor(null)
    }

    const lineThreads =
      threads && filePath ? getThreadsForLine(threads, filePath, line) : []

    // Outdated threads: lines in threads that no longer match any live line
    // are passed with isOutdated=true. Since this component only renders if the
    // line exists in the current diff, threads here are always "live".
    // Outdated threads are rendered in a separate pass in TextDiffViewer (below).

    if (viewMode === 'split' && splitSide) {
      const isLeft = splitSide === 'left'
      const isVisible = isLeft ? line.line_type !== 'added' : line.line_type !== 'removed'
      const gutterNumber = isLeft ? line.old_line_number : line.new_line_number
      const diffType = isLeft
        ? line.line_type === 'removed'
          ? 'removed'
          : 'context'
        : line.line_type === 'added'
          ? 'added'
          : 'context'

      return (
        <div className={cn('group/diffrow flex min-w-max', getLineBackground(diffType))} style={rowStyle}>
          <div
            className={cn(
              'relative w-11 shrink-0 select-none border-border border-r px-2 py-0.5 text-right',
              'editor-font code-editor-font-override text-muted-foreground tabular-nums',
              getGutterBackground(diffType),
            )}
            style={gutterStyle}
          >
            {isVisible ? gutterNumber : ''}
            {canComment && isVisible && <AddCommentButton onClick={handleAddComment} />}
          </div>
          <div
            className={cn(
              'editor-font code-editor-font-override m-0 min-w-0 flex-1 px-2.5 py-0.5 antialiased',
              diffType === 'added'
                ? getContentColor('added')
                : diffType === 'removed'
                  ? getContentColor('removed')
                  : 'text-foreground',
            )}
            style={contentStyle}
          >
            {isVisible ? lineContent : ''}
          </div>
        </div>
      )
    }

    if (viewMode === 'split') {
      return (
        <div className="group/diffrow flex min-w-0 w-full" style={rowStyle}>
          <div
            className={cn(
              'flex min-h-0 min-w-0 basis-1/2 overflow-hidden border-border border-r',
              line.line_type === 'removed' ? getLineBackground('removed') : '',
            )}
          >
            <div
              className={cn(
                'relative w-11 shrink-0 select-none border-border border-r px-2 py-0.5 text-right',
                'editor-font code-editor-font-override text-muted-foreground tabular-nums',
                getGutterBackground(line.line_type === 'removed' ? 'removed' : ''),
              )}
              style={gutterStyle}
            >
              {line.line_type !== 'added' ? line.old_line_number : ''}
              {canComment && line.line_type !== 'added' && (
                <AddCommentButton onClick={handleAddComment} />
              )}
            </div>
            <div
              className={cn(
                'editor-font code-editor-font-override m-0 min-w-0 flex-1 overflow-x-auto overflow-y-hidden px-2.5 py-0.5 antialiased',
                line.line_type === 'removed' ? getContentColor('removed') : 'text-foreground',
              )}
              style={contentStyle}
            >
              {line.line_type !== 'added' ? lineContent : ''}
            </div>
          </div>

          <div
            className={cn(
              'flex min-h-0 min-w-0 basis-1/2 overflow-hidden',
              line.line_type === 'added' ? getLineBackground('added') : '',
            )}
          >
            <div
              className={cn(
                'relative w-11 shrink-0 select-none border-border border-r px-2 py-0.5 text-right',
                'editor-font code-editor-font-override text-muted-foreground tabular-nums',
                getGutterBackground(line.line_type === 'added' ? 'added' : ''),
              )}
              style={gutterStyle}
            >
              {line.line_type !== 'removed' ? line.new_line_number : ''}
              {canComment && line.line_type !== 'removed' && (
                <AddCommentButton onClick={handleAddComment} />
              )}
            </div>
            <div
              className={cn(
                'editor-font code-editor-font-override m-0 min-w-0 flex-1 overflow-x-auto overflow-y-hidden px-2.5 py-0.5 antialiased',
                line.line_type === 'added' ? getContentColor('added') : 'text-foreground',
              )}
              style={contentStyle}
            >
              {line.line_type !== 'removed' ? lineContent : ''}
            </div>
          </div>
        </div>
      )
    }

    // Unified view
    return (
      <>
        <div
          className={cn('group/diffrow relative flex min-w-full w-fit', getLineBackground(line.line_type))}
          style={rowStyle}
        >
          <div
            className={cn(
              'relative w-11 shrink-0 select-none border-border border-r px-2 py-0.5 text-right',
              'editor-font code-editor-font-override text-muted-foreground tabular-nums',
              getGutterBackground(line.line_type),
            )}
            style={gutterStyle}
          >
            {line.old_line_number}
            {canComment && line.line_type === 'removed' && (
              <AddCommentButton onClick={handleAddComment} />
            )}
          </div>
          <div
            className={cn(
              'relative w-11 shrink-0 select-none border-border border-r px-2 py-0.5 text-right',
              'editor-font code-editor-font-override text-muted-foreground tabular-nums',
              getGutterBackground(line.line_type),
            )}
            style={gutterStyle}
          >
            {line.new_line_number}
            {canComment && line.line_type !== 'removed' && (
              <AddCommentButton onClick={handleAddComment} />
            )}
          </div>

          <div
            className={cn(
              'editor-font code-editor-font-override m-0 min-w-0 flex-1 px-2.5 py-0.5 antialiased',
              getContentColor(line.line_type),
            )}
            style={contentStyle}
          >
            {lineContent}
          </div>
        </div>

        {/* Inline composer — opens below the line when "+" is clicked */}
        {composerAnchor && wsId && (
          <ComposerRow
            anchor={composerAnchor}
            wsId={wsId}
            onSubmit={handleComposerSubmit}
            onCancel={() => setComposerAnchor(null)}
          />
        )}

        {/* Thread rows anchored to this line */}
        {wsId && lineThreads.length > 0 && (
          <ThreadRows threads={lineThreads} wsId={wsId} />
        )}
      </>
    )
  },
)

DiffLine.displayName = 'DiffLine'

export default DiffLine

/**
 * Outdated thread rows: threads whose anchor line no longer appears in the diff.
 * Rendered once at the bottom of the file diff, with isOutdated=true.
 */
export function OutdatedThreadRows({
  threads,
  wsId,
}: {
  threads: ReviewThread[]
  wsId: string
}) {
  const handleReply = async (threadId: string, body: string) => {
    await replyToThread(wsId, threadId, { body })
  }
  const handleResolve = async (threadId: string) => {
    await setThreadResolved(wsId, threadId, true)
  }
  const handleReopen = async (threadId: string) => {
    await setThreadResolved(wsId, threadId, false)
  }

  if (threads.length === 0) return null

  return (
    <div className="border-border border-t bg-muted/10 px-2 py-1">
      {threads.map((thread) => (
        <ReviewThreadItem
          key={thread.id}
          thread={thread}
          wsId={wsId}
          isOutdated
          onReply={handleReply}
          onResolve={handleResolve}
          onReopen={handleReopen}
        />
      ))}
    </div>
  )
}
