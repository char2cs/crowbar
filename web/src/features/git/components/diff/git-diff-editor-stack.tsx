import {
  CaretDown as ChevronDown,
  CaretRight as ChevronRight,
  Columns as Columns2,
  ArrowSquareOut as ExternalLink,
  Rows as Rows3,
} from '@phosphor-icons/react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
const openUrl = (url: string) => {
  window.open(url, '_blank')
}
import CodeEditor from '@/features/editor/components/code-editor'
import { useHunkStagingZones } from './use-hunk-staging-zones'
import { useReviewCommentLayer } from './use-review-comment-layer'
import Breadcrumb from '@/features/editor/components/toolbar/breadcrumb'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { useWorkspaceStore, useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useEditorSettingsStore } from '@/features/editor/stores/settings-store'
import { calculateLineHeight, splitLines } from '@/features/editor/utils/lines'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { Button } from '@/components/ui/button'
import Tooltip from '@/components/ui/tooltip'
import { cn } from '@/utils/cn'
import { DiffReviewHeader } from './diff-review-header'
import { DiffSearchBar } from './diff-search-bar'
import { DiffSearchProvider, useDiffSearchContext } from './diff-search-context'
import { useDiffSearch } from './use-diff-search'
import { getRemotes } from '../../api/git-remotes-api'
import { getGitStatus } from '../../api/git-status-api'
import { useDiffEditorBuffer } from '../../hooks/use-diff-editor-buffer'
import type { MultiFileDiff } from '../../types/git-diff-types'
import type { GitDiff } from '../../types/git-types'
import { gitDiffCache } from '../../utils/git-diff-cache'
import { getFileStatus } from '../../utils/git-diff-helpers'
import {
  getInitialExpandedDiffFileKeys,
  shouldUseScrollableDiffEditor,
} from '../../utils/diff-viewer-scale'
import { buildWorkingTreeMultiDiff } from '../../utils/working-tree-multi-diff'
import {
  serializeGitDiffForEditor,
  serializeGitDiffSourceForEditor,
  serializeGitDiffSourceForSplitEditor,
} from '../../utils/diff-editor-content'
import GitDiffEditorSurface from './git-diff-editor-surface'
import ImageDiffViewer from './git-diff-image'
import { Badge } from '@/components/ui/badge'

function countStats(diff: GitDiff) {
  if (typeof diff.additions === 'number' || typeof diff.deletions === 'number') {
    return {
      additions: diff.additions ?? 0,
      deletions: diff.deletions ?? 0,
    }
  }

  let additions = 0
  let deletions = 0

  for (const line of diff.lines) {
    if (line.line_type === 'added') additions++
    if (line.line_type === 'removed') deletions++
  }

  return { additions, deletions }
}

const statusTextClass: Record<string, string> = {
  added: 'text-git-added',
  deleted: 'text-git-deleted',
  modified: 'text-git-modified',
  renamed: 'text-git-renamed',
}

const statusBadgeClass: Record<string, string> = {
  added: 'bg-git-added/12 text-git-added',
  deleted: 'bg-git-deleted/12 text-git-deleted',
  modified: 'bg-git-modified/12 text-git-modified',
  renamed: 'bg-git-renamed/12 text-git-renamed',
}


function parseGitHubRemoteSlug(remoteUrl: string): { owner: string; repo: string } | null {
  const normalized = remoteUrl.trim()
  const httpsMatch = normalized.match(/^https?:\/\/github\.com\/([^/]+)\/([^/]+?)(?:\.git)?\/?$/i)
  if (httpsMatch) {
    const [, owner, repo] = httpsMatch
    return { owner, repo }
  }

  const sshMatch = normalized.match(/^git@github\.com:([^/]+)\/(.+?)(?:\.git)?$/i)
  if (sshMatch) {
    const [, owner, repo] = sshMatch
    return { owner, repo }
  }

  return null
}

function buildGitHubReferenceUrl(remoteUrl: string, gitRef: string): string | null {
  const slug = parseGitHubRemoteSlug(remoteUrl)
  if (!slug) return null

  const comparisonMatch = gitRef.match(/^(.+?)(?:\.{2,3})(.+)$/)
  if (comparisonMatch) {
    const [, baseRef, targetRef] = comparisonMatch
    return `https://github.com/${slug.owner}/${slug.repo}/compare/${encodeURIComponent(
      baseRef,
    )}...${encodeURIComponent(targetRef)}`
  }

  return `https://github.com/${slug.owner}/${slug.repo}/commit/${encodeURIComponent(gitRef)}`
}

function LargeDiffSectionEditor({ diff, cacheKey }: { diff: GitDiff; cacheKey: string }) {
  const containerStyle = {
    height: 'min(72vh, 760px)',
    minHeight: '420px',
  } as const

  // Hoist all hooks above any early returns (Rules of Hooks)
  const sourcePath = diff.new_path || diff.old_path || diff.file_path
  const editorContent = useMemo(() => serializeGitDiffForEditor(diff), [diff])
  const bufferId = useDiffEditorBuffer({
    cacheKey: `${cacheKey}_large`,
    content: editorContent,
    sourcePath,
    name: `${sourcePath.split('/').pop() || 'Diff'}.diff`,
  })

  // raw_patch diffs have no parsed lines; fall back to plain text display
  if (diff.raw_patch && diff.lines.length === 0) {
    return (
      <div
        className="relative overflow-hidden border-border border-t bg-transparent"
        style={containerStyle}
      >
        <GitDiffEditorSurface cacheKey={`${cacheKey}_raw`} diff={diff} />
      </div>
    )
  }

  return (
    <div
      className="relative overflow-hidden border-border border-t bg-transparent"
      style={containerStyle}
    >
      <CodeEditor
        bufferId={bufferId}
        isActiveSurface={false}
        showToolbar={false}
        readOnly={true}
        scrollable={true}
      />
    </div>
  )
}

function EmbeddedDiffSectionEditor({
  diff,
  cacheKey,
  viewMode,
  enableComments = false,
  enableHunkActions = false,
}: {
  diff: GitDiff
  cacheKey: string
  viewMode: 'unified' | 'split'
  enableComments?: boolean
  enableHunkActions?: boolean
}) {
  const fontSize = useEditorSettingsStore.use.fontSize()
  const zoomLevel = useZoomStore.use.editorZoomLevel()
  const sourcePath = diff.new_path || diff.old_path || diff.file_path
  // Diff-search highlights for THIS file (unified editor): its matches + the
  // active match when it belongs here.
  const searchLayer = useDiffSearchContext()
  const fileSearchMatches = searchLayer?.matchesByFile.get(cacheKey) ?? null
  const activeSearchMatch =
    searchLayer?.active && searchLayer.active.fileKey === cacheKey ? searchLayer.active : null
  const unifiedContent = useMemo(() => serializeGitDiffSourceForEditor(diff), [diff])
  const splitContent = useMemo(() => serializeGitDiffSourceForSplitEditor(diff), [diff])
  // Inline review comments and per-hunk staging are both hosted as Monaco view
  // zones on the unified surface (split would need per-side zones).
  const commentLayer = useReviewCommentLayer({
    enabled: enableComments && viewMode === 'unified',
    diff,
    unifiedLineKinds: unifiedContent.lineKinds,
  })
  const hunkZones = useHunkStagingZones({
    enabled: enableHunkActions && viewMode === 'unified',
    diff,
    isStaged: cacheKey.startsWith('staged:'),
  })
  // One zone layer feeds the editor: review comment threads and/or hunk headers.
  const inlineZones = useMemo(
    () => [...(commentLayer?.commentZones ?? []), ...(hunkZones ?? [])],
    [commentLayer, hunkZones],
  )
  const hasInlineLayer = commentLayer != null || hunkZones != null
  const [commentContentHeight, setCommentContentHeight] = useState<number | null>(null)
  const unifiedBufferId = useDiffEditorBuffer({
    cacheKey,
    content: unifiedContent.content,
    sourcePath,
    name: sourcePath.split('/').pop() || 'Diff',
    pathOverride: sourcePath,
  })
  const leftSplitBufferId = useDiffEditorBuffer({
    cacheKey: `${cacheKey}_left`,
    content: splitContent.left.content,
    sourcePath,
    name: `${sourcePath.split('/').pop() || 'Diff'} (left)`,
    pathOverride: sourcePath,
  })
  const rightSplitBufferId = useDiffEditorBuffer({
    cacheKey: `${cacheKey}_right`,
    content: splitContent.right.content,
    sourcePath,
    name: `${sourcePath.split('/').pop() || 'Diff'} (right)`,
    pathOverride: sourcePath,
  })
  const height = useMemo(() => {
    const lineCount =
      viewMode === 'split'
        ? Math.max(
            splitLines(splitContent.left.content).length,
            splitLines(splitContent.right.content).length,
          )
        : splitLines(unifiedContent.content).length
    const lineHeight = calculateLineHeight(fontSize * zoomLevel)

    return Math.max(
      lineCount * lineHeight +
        EDITOR_CONSTANTS.EDITOR_PADDING_TOP +
        EDITOR_CONSTANTS.EDITOR_PADDING_BOTTOM,
      160,
    )
  }, [
    fontSize,
    splitContent.left.content,
    splitContent.right.content,
    unifiedContent.content,
    viewMode,
    zoomLevel,
  ])

  if (viewMode === 'split') {
    return (
      <div
        className="grid grid-cols-2 border-border border-t bg-transparent"
        style={{ height: `${height}px` }}
      >
        <div className="relative overflow-hidden border-border border-r bg-transparent">
          <CodeEditor
            bufferId={leftSplitBufferId}
            isActiveSurface={false}
            showToolbar={false}
            readOnly={true}
            scrollable={false}
            diffLineKinds={splitContent.left.lineKinds}
          />
        </div>
        <div className="relative overflow-hidden bg-transparent">
          <CodeEditor
            bufferId={rightSplitBufferId}
            isActiveSurface={false}
            showToolbar={false}
            readOnly={true}
            scrollable={false}
            diffLineKinds={splitContent.right.lineKinds}
          />
        </div>
      </div>
    )
  }

  // Inline zones (comments / hunk headers) push lines down, so grow the section
  // to the editor's true content height. The tint is always applied as Monaco
  // whole-line decorations (part of the editor layout) — a position-based CSS
  // overlay drifts out of alignment with the rendered lines.
  // Add breathing room below the last inline thread so it isn't flush against
  // the section's bottom edge (which reads as "cut off").
  const unifiedHeight =
    hasInlineLayer && commentContentHeight
      ? Math.max(height, commentContentHeight + 20)
      : height

  return (
    <>
      <div
        className="relative overflow-hidden border-border border-t bg-transparent"
        style={{ height: `${unifiedHeight}px` }}
      >
        <CodeEditor
          bufferId={unifiedBufferId}
          isActiveSurface={false}
          showToolbar={false}
          readOnly={true}
          scrollable={false}
          commentZones={hasInlineLayer ? inlineZones : undefined}
          onAddCommentAtLine={commentLayer?.onAddCommentAtLine}
          onContentHeightChange={hasInlineLayer ? setCommentContentHeight : undefined}
          diffLineKinds={unifiedContent.lineKinds}
          diffSearchMatches={fileSearchMatches}
          activeDiffSearchMatch={activeSearchMatch}
          diffSearchRevealNonce={searchLayer?.revealNonce}
        />
      </div>
      {commentLayer?.commentError && (
        <div className="border-border border-t px-3 py-1.5 text-xs text-destructive">
          {commentLayer.commentError.msg}
        </div>
      )}
    </>
  )
}

function DiffSectionEditor({
  diff,
  cacheKey,
  viewMode,
  enableComments = false,
  enableHunkActions = false,
}: {
  diff: GitDiff
  cacheKey: string
  viewMode: 'unified' | 'split'
  enableComments?: boolean
  enableHunkActions?: boolean
}) {
  if (shouldUseScrollableDiffEditor(diff)) {
    return <LargeDiffSectionEditor diff={diff} cacheKey={cacheKey} />
  }

  return (
    <EmbeddedDiffSectionEditor
      diff={diff}
      cacheKey={cacheKey}
      viewMode={viewMode}
      enableComments={enableComments}
      enableHunkActions={enableHunkActions}
    />
  )
}

const LazyDiffSectionBody = memo(function LazyDiffSectionBody({
  expanded,
  children,
}: {
  expanded: boolean
  children: React.ReactNode
}) {
  const bodyRef = useRef<HTMLDivElement>(null)
  const [shouldMount, setShouldMount] = useState(expanded)

  useEffect(() => {
    if (!expanded) {
      setShouldMount(false)
      return
    }

    const element = bodyRef.current
    if (!element) {
      setShouldMount(true)
      return
    }

    const scrollContainer = element.closest('[data-diff-stack-scroll-container]')
    if (!(scrollContainer instanceof HTMLDivElement)) {
      setShouldMount(true)
      return
    }

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0]
        if (entry?.isIntersecting) {
          setShouldMount(true)
          observer.disconnect()
        }
      },
      {
        root: scrollContainer,
        rootMargin: '1200px 0px',
      },
    )

    observer.observe(element)
    return () => observer.disconnect()
  }, [expanded])

  return (
    <div
      ref={bodyRef}
      className="border-border border-t"
      style={{ contentVisibility: 'auto', containIntrinsicSize: '960px' }}
    >
      {shouldMount ? children : <div className="h-[320px] bg-transparent" />}
    </div>
  )
})

const DiffFileSection = memo(function DiffFileSection({
  diff,
  sectionKey,
  expanded,
  onToggle,
  viewMode,
  enableHunkActions,
  enableComments,
  onOpenFile,
}: {
  diff: GitDiff
  sectionKey: string
  expanded: boolean
  onToggle: (sectionKey: string) => void
  onOpenFile: (filePath: string) => void | Promise<void>
  viewMode: 'unified' | 'split'
  enableHunkActions: boolean
  enableComments: boolean
}) {
  const filePath = diff.new_path || diff.old_path || diff.file_path
  const fileName = filePath.split('/').pop() || filePath
  const directoryPath = filePath.includes('/')
    ? filePath.slice(0, filePath.lastIndexOf('/') + 1)
    : ''
  const status = getFileStatus(diff) as 'added' | 'deleted' | 'modified' | 'renamed'
  const { additions, deletions } = countStats(diff)
  const handleToggle = useCallback(() => {
    onToggle(sectionKey)
  }, [onToggle, sectionKey])
  const handleOpenFile = useCallback(() => {
    void onOpenFile(filePath)
  }, [filePath, onOpenFile])

  return (
    <section className="relative isolate min-w-0 max-w-full rounded-md bg-transparent">
      <div className="min-w-0 max-w-full bg-transparent">
        <div
          className={cn(
            'min-w-0 max-w-full overflow-hidden border border-border/70 bg-transparent shadow-[0_1px_0_rgba(0,0,0,0.04)]',
            expanded ? 'rounded-t-md' : 'rounded-md',
          )}
        >
          <div className="flex min-w-0 items-center">
            <button
              type="button"
              onClick={handleToggle}
              className="relative z-50 flex h-8 w-8 shrink-0 items-center justify-center text-muted-foreground hover:bg-muted/30 hover:text-foreground"
              aria-label={expanded ? 'Collapse file diff' : 'Expand file diff'}
              aria-expanded={expanded}
            >
              {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
            </button>
            <button
              type="button"
              onClick={handleOpenFile}
              className="relative z-50 flex min-w-0 flex-1 items-center gap-2 overflow-hidden py-2 pr-3 text-left hover:bg-muted/30"
              aria-label={`Open ${filePath}`}
            >
              <FileExplorerIcon
                fileName={fileName}
                isDir={false}
                size={16}
                className="shrink-0 text-muted-foreground"
              />
              <span className="flex min-w-0 flex-1 items-baseline gap-2 overflow-hidden">
                <span
                  className={cn(
                    'min-w-0 max-w-[45%] truncate ui-text-sm font-medium',
                    statusTextClass[status],
                  )}
                >
                  {fileName}
                </span>
                <span className="min-w-0 flex-1 truncate ui-text-sm editor-font text-muted-foreground">
                  {directoryPath}
                </span>
              </span>
              <span className="ml-auto flex shrink-0 items-center gap-2 ui-text-xs">
                {additions > 0 ? <span className="text-git-added">+{additions}</span> : null}
                {deletions > 0 ? <span className="text-git-deleted">-{deletions}</span> : null}
                <Badge
                  size="sm"
                  variant="secondary"
                  className={`rounded px-1.5 py-0.5 capitalize ${statusBadgeClass[status]}`}
                >
                  {status}
                </Badge>
              </span>
            </button>
          </div>
        </div>
      </div>

      {expanded ? (
        diff.is_image ? (
          <div className="-mt-px min-w-0 max-w-full overflow-hidden rounded-b-md border-border/70 border-x border-b">
            <LazyDiffSectionBody expanded={expanded}>
              <ImageDiffViewer diff={diff} fileName={fileName} onClose={() => {}} />
            </LazyDiffSectionBody>
          </div>
        ) : (
          <div className="-mt-px min-w-0 max-w-full overflow-hidden rounded-b-md border-border/70 border-x border-b">
            <LazyDiffSectionBody expanded={expanded}>
              <DiffSectionEditor
                diff={diff}
                cacheKey={sectionKey}
                viewMode={viewMode}
                enableComments={enableComments}
                enableHunkActions={enableHunkActions}
              />
            </LazyDiffSectionBody>
          </div>
        )
      ) : null}
    </section>
  )
})

function getInitialExpandedFiles(multiDiff: MultiFileDiff): Set<string> {
  return new Set(getInitialExpandedDiffFileKeys(multiDiff))
}

const GitDiffEditorStack = memo(function GitDiffEditorStack({
  multiDiff,
  enableComments = false,
  branchHeader,
  isActivePane = true,
}: {
  multiDiff: MultiFileDiff
  /** Enable the inline review-comment layer (the Branch Review surface). */
  enableComments?: boolean
  /**
   * Branch-review header data. When present, the shared header shows the branch
   * name as its title and the base branch as its meta (in place of the
   * commit message + author/date/hash a commit diff would show).
   */
  branchHeader?: { title: string; baseBranch?: string }
  /**
   * Whether this diff's pane is the active one. The find flag (`isFindVisible`)
   * is global and shared with the text-editor find, so the diff only opens its
   * own search bar when it is the active pane — otherwise a split with an active
   * editor + an inactive diff would pop two find UIs from one Cmd+F.
   */
  isActivePane?: boolean
}) {
  const workspaceStore = useWorkspaceStore()
  const buffers = useStore(workspaceStore, (s) => s.buffers)
  const activeBufferId = useStore(
    workspaceStore,
    (s) => s.paneActions.getActivePane()?.activeBufferId ?? null,
  )
  const updateBufferContent = (
    bufferId: string,
    content: string,
    _markDirty: boolean,
    diffData?: MultiFileDiff,
  ) => {
    workspaceStore.setState((state) => ({
      ...state,
      buffers: state.buffers.map((b) =>
        b.id === bufferId && b.type === 'diff'
          ? { ...b, content, ...(diffData !== undefined ? { diffData } : {}) }
          : b,
      ),
    }))
  }
  const rootFolderPath = useFileSystemStore((state) => state.rootFolderPath)
  const [viewMode, setViewMode] = useState<'unified' | 'split'>('unified')
  const isWorkingTree = multiDiff.commitHash === 'working-tree'
  const activeBuffer = buffers.find((buffer) => buffer.id === activeBufferId) || null
  const isWorkingTreeBuffer = activeBuffer?.path === 'diff://working-tree/all-files'
  const isRefreshingRef = useRef(false)
  const handleOpenFile = useCallback(async (filePath: string) => {
    // Diff file paths are workspace-relative; handleFileSelect → openFileContent
    // resolves them within the workspace. Joining with a repo root (the old code)
    // produced a non-workspace path that failed to open.
    useFileSystemStore.getState().handleFileSelect?.(filePath, false)
  }, [])
  const [githubCommitUrl, setGitHubCommitUrl] = useState<string | null>(null)
  const [expandedFiles, setExpandedFiles] = useState<Set<string>>(() =>
    getInitialExpandedFiles(multiDiff),
  )
  const handleToggleSection = useCallback((sectionKey: string) => {
    setExpandedFiles((prev) => {
      const next = new Set(prev)
      if (next.has(sectionKey)) next.delete(sectionKey)
      else next.add(sectionKey)
      return next
    })
  }, [])

  const keyForIndex = useCallback(
    (index: number) =>
      multiDiff.fileKeys?.[index] ?? `${multiDiff.files[index]?.file_path}:${index}`,
    [multiDiff.fileKeys, multiDiff.files],
  )

  // Virtualize the file list — an 830-file diff otherwise renders every section
  // (each with a sticky header), which the browser cannot keep up with. Only the
  // sections near the viewport mount; heights are measured dynamically.
  const scrollRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: multiDiff.files.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => {
      const diff = multiDiff.files[index]
      if (!diff) return 44
      if (!expandedFiles.has(keyForIndex(index))) return 44
      if (shouldUseScrollableDiffEditor(diff)) return 620
      return 44 + Math.max(diff.lines.length, 1) * 20 + 8
    },
    overscan: 3,
    measureElement: (el) => el.getBoundingClientRect().height,
  })

  // ── Diff-wide search ──────────────────────────────────────────────────────
  // The search button + Cmd/Ctrl+F toggle the global find flag; the diff opens
  // its own find bar in response and searches across ALL files.
  const isFindVisible = useUIState((s) => s.isFindVisible)
  const setIsFindVisible = useUIState((s) => s.setIsFindVisible)
  // Diff search is for the surfaces you actively work with — the branch review
  // and the working tree. A commit/stash/tag diff is a historical snapshot
  // (identified by a real commitHash); search is disabled there.
  const isCommitDiff = !isWorkingTree && Boolean(multiDiff.commitHash)
  // Only this pane responds to the global find flag when it is active.
  const searchOpen = isFindVisible && isActivePane && !isCommitDiff
  const search = useDiffSearch({
    files: multiDiff.files,
    keyForIndex,
    enabled: searchOpen,
  })
  const searchContextValue = useMemo(
    () => ({
      matchesByFile: search.matchesByFile,
      active: search.current,
      revealNonce: search.revealNonce,
    }),
    [search.matchesByFile, search.current, search.revealNonce],
  )
  // On navigation / new results, bring the active match's file into view; its
  // editor then reveals the exact line (see monaco-diff-editor reveal effect).
  useEffect(() => {
    const current = search.current
    if (!current) return
    setExpandedFiles((prev) => (prev.has(current.fileKey) ? prev : new Set(prev).add(current.fileKey)))
    virtualizer.scrollToIndex(current.fileIndex, { align: 'start' })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search.revealNonce])
  const closeSearch = useCallback(() => setIsFindVisible(false), [setIsFindVisible])
  // Search highlights + line reveal are wired for the unified editor only; switch
  // away from split when the search opens so matches are actually shown.
  useEffect(() => {
    if (searchOpen && viewMode === 'split') setViewMode('unified')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchOpen])

  // Scroll-to-file: the Branch Review side panel drives this via activeFileKey/Nonce.
  const activeReviewFileKey = useWorkspaceStoreContext((s) => s.branchReview.activeFileKey)
  const activeReviewFileNonce = useWorkspaceStoreContext((s) => s.branchReview.activeFileNonce)
  useEffect(() => {
    if (!activeReviewFileKey || activeReviewFileNonce === 0) return
    const index = multiDiff.files.findIndex((_, i) => keyForIndex(i) === activeReviewFileKey)
    if (index === -1) return
    setExpandedFiles((prev) => new Set(prev).add(activeReviewFileKey))
    virtualizer.scrollToIndex(index, { align: 'start' })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeReviewFileNonce])

  useEffect(() => {
    const nextKeys = new Set(
      multiDiff.files.map(
        (diff, index) => multiDiff.fileKeys?.[index] ?? `${diff.file_path}:${index}`,
      ),
    )

    setExpandedFiles((previous) => {
      const nextExpanded = new Set(Array.from(previous).filter((key) => nextKeys.has(key)))

      if (nextExpanded.size === 0) {
        return getInitialExpandedFiles(multiDiff)
      }

      if (multiDiff.initiallyExpandedFileKey && nextKeys.has(multiDiff.initiallyExpandedFileKey)) {
        nextExpanded.add(multiDiff.initiallyExpandedFileKey)
      }

      return nextExpanded
    })
  }, [multiDiff.fileKeys, multiDiff.files, multiDiff.initiallyExpandedFileKey])

  const refreshWorkingTreeBuffer = useCallback(async () => {
    if (!isWorkingTree || !isWorkingTreeBuffer || !rootFolderPath || !activeBuffer) return
    if (isRefreshingRef.current) return

    isRefreshingRef.current = true

    try {
      gitDiffCache.invalidate(rootFolderPath)
      const gitStatus = await getGitStatus(rootFolderPath)
      const nextMultiDiff = await buildWorkingTreeMultiDiff({
        repoPath: rootFolderPath,
        status: gitStatus,
        previousFileKeys: multiDiff.fileKeys,
      })

      // A clean tree keeps the tab open with an explicit empty state — the
      // committed hunks must never linger after the working tree is clean.
      updateBufferContent(activeBuffer.id, '', false, nextMultiDiff)
    } finally {
      isRefreshingRef.current = false
    }
  }, [
    activeBuffer,
    isWorkingTree,
    isWorkingTreeBuffer,
    multiDiff.fileKeys,
    rootFolderPath,
    updateBufferContent,
  ])

  useEffect(() => {
    if (!isWorkingTree) return

    const handleGitStatusChanged = () => {
      window.setTimeout(() => {
        void refreshWorkingTreeBuffer()
      }, 50)
    }

    window.addEventListener('git-status-changed', handleGitStatusChanged)
    return () => {
      window.removeEventListener('git-status-changed', handleGitStatusChanged)
    }
  }, [isWorkingTree, refreshWorkingTreeBuffer])

  useEffect(() => {
    // The branch-review diff has no single commitHash (it's a branch comparison),
    // so there is no GitHub commit URL to build.
    if (isWorkingTree || !multiDiff.commitHash || multiDiff.commitHash.startsWith('stash@{')) {
      setGitHubCommitUrl(null)
      return
    }

    const repoPath = multiDiff.repoPath ?? rootFolderPath
    if (!repoPath) {
      setGitHubCommitUrl(null)
      return
    }

    let isCancelled = false

    const loadGitHubCommitUrl = async () => {
      const remotes = await getRemotes(repoPath)
      const candidate =
        remotes.find((remote) => remote.name === 'origin')?.url ?? remotes[0]?.url ?? null
      const nextUrl = candidate ? buildGitHubReferenceUrl(candidate, multiDiff.commitHash) : null
      if (!isCancelled) {
        setGitHubCommitUrl(nextUrl)
      }
    }

    void loadGitHubCommitUrl()

    return () => {
      isCancelled = true
    }
  }, [isWorkingTree, multiDiff.commitHash, multiDiff.repoPath, rootFolderPath])

  return (
    <div className="flex h-full flex-col overflow-hidden bg-transparent">
      <Breadcrumb
        filePathOverride={multiDiff.title || 'Uncommitted Changes'}
        interactive={false}
        showPath={false}
        showDefaultActions={true}
        extraLeftContent={
          <div className="ui-text-sm flex items-center gap-2 text-muted-foreground">
            <span>
              {multiDiff.totalFiles} changed file
              {multiDiff.totalFiles !== 1 ? 's' : ''}
            </span>
            <span className="text-git-added">+{multiDiff.totalAdditions}</span>
            <span className="text-git-deleted">-{multiDiff.totalDeletions}</span>
          </div>
        }
        rightContent={
          <div className="flex items-center gap-1">
            {githubCommitUrl ? (
              <Tooltip content="View on GitHub" side="bottom">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => void openUrl(githubCommitUrl)}
                  className="h-5 gap-1 px-1.5 text-muted-foreground ui-text-sm"
                  aria-label="View on GitHub"
                >
                  <ExternalLink />
                  View on GitHub
                </Button>
              </Tooltip>
            ) : null}
            <div className="flex items-center gap-0.5">
              <Tooltip content="Unified view" side="bottom">
                <Button
                  type="button"
                  variant="ghost"
                  active={viewMode === 'unified'}
                  onClick={() => setViewMode('unified')}
                  className="text-muted-foreground"
                  aria-label="Unified view"
                >
                  <Rows3 />
                </Button>
              </Tooltip>
              <Tooltip content="Split view" side="bottom">
                <Button
                  type="button"
                  variant="ghost"
                  active={viewMode === 'split'}
                  onClick={() => setViewMode('split')}
                  className="text-muted-foreground"
                  aria-label="Split view"
                >
                  <Columns2 />
                </Button>
              </Tooltip>
            </div>
          </div>
        }
      />

      {branchHeader ? (
        <DiffReviewHeader title={branchHeader.title} baseBranch={branchHeader.baseBranch} />
      ) : !isWorkingTree &&
        (multiDiff.commitMessage || multiDiff.commitAuthor || multiDiff.commitDate) ? (
        <DiffReviewHeader
          title={multiDiff.commitMessage}
          description={multiDiff.commitDescription}
          author={multiDiff.commitAuthor}
          date={multiDiff.commitDate}
          hash={multiDiff.commitHash}
        />
      ) : null}

      {searchOpen ? <DiffSearchBar search={search} onClose={closeSearch} /> : null}

      <DiffSearchProvider value={searchContextValue}>
      <div
        ref={scrollRef}
        className="min-h-0 flex-1 overflow-auto px-2 pb-2"
        style={{ overflowAnchor: 'none' }}
        data-diff-stack-scroll-container
      >
        {isWorkingTree && multiDiff.files.length === 0 && !multiDiff.isLoading ? (
          <div className="flex h-full items-center justify-center text-[13px] text-muted-foreground">
            No uncommitted changes
          </div>
        ) : null}
        <div
          className="relative min-w-0 max-w-full"
          style={{ height: `${virtualizer.getTotalSize()}px` }}
        >
          {virtualizer.getVirtualItems().map((virtualItem) => {
            const diff = multiDiff.files[virtualItem.index]
            if (!diff) return null
            const sectionKey = keyForIndex(virtualItem.index)

            return (
              <div
                key={sectionKey}
                ref={virtualizer.measureElement}
                data-index={virtualItem.index}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${virtualItem.start}px)`,
                  paddingBottom: 8,
                }}
              >
                <DiffFileSection
                  diff={diff}
                  sectionKey={sectionKey}
                  expanded={expandedFiles.has(sectionKey)}
                  viewMode={viewMode}
                  enableHunkActions={isWorkingTree}
                  enableComments={enableComments}
                  onToggle={handleToggleSection}
                  onOpenFile={handleOpenFile}
                />
              </div>
            )
          })}
        </div>
      </div>
      </DiffSearchProvider>
    </div>
  )
})

export default GitDiffEditorStack
