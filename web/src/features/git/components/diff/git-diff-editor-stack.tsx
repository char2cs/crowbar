import {
  Check,
  CaretDown as ChevronDown,
  CaretRight as ChevronRight,
  Columns as Columns2,
  ArrowSquareOut as ExternalLink,
  Rows as Rows3,
  Trash as Trash2,
} from "@phosphor-icons/react";
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "zustand";
const openUrl = (url: string) => { window.open(url, "_blank") }
import Breadcrumb from "@/features/editor/components/toolbar/breadcrumb";
import { FileExplorerIcon } from "@/features/file-explorer/components/file-explorer-icon";
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";
import { useEditorSettingsStore } from "@/features/editor/stores/settings-store";
import { calculateLineHeight, splitLines } from "@/features/editor/utils/lines";
import { useZoomStore } from "@/features/window/stores/zoom-store";
import { useFileSystemStore } from "@/features/file-system/controllers/store";
import { Button } from "@/components/ui/button";
import Tooltip from "@/components/ui/tooltip";
import { cn } from "@/utils/cn";
import { formatRelativeDate } from "@/utils/date";
import { joinPath } from "@/utils/path-helpers";
import { getRemotes } from "../../api/git-remotes-api";
import { getGitStatus } from "../../api/git-status-api";
import type { MultiFileDiff } from "../../types/git-diff-types";
import type { GitDiff } from "../../types/git-types";
import { gitDiffCache } from "../../utils/git-diff-cache";
import { getFileStatus } from "../../utils/git-diff-helpers";
import {
  getInitialExpandedDiffFileKeys,
  shouldUseScrollableDiffEditor,
} from "../../utils/diff-viewer-scale";
import { buildWorkingTreeMultiDiff } from "../../utils/working-tree-multi-diff";
import { buildMonacoDiffContent } from "../../utils/diff-editor-content";
import MonacoDiffEditorView from "./monaco-diff-editor-view";
import ImageDiffViewer from "./git-diff-image";
import TextDiffViewer from "./git-diff-text";
import { Badge } from "@/components/ui/badge";

function countStats(diff: GitDiff) {
  if (typeof diff.additions === "number" || typeof diff.deletions === "number") {
    return {
      additions: diff.additions ?? 0,
      deletions: diff.deletions ?? 0,
    };
  }

  let additions = 0;
  let deletions = 0;

  for (const line of diff.lines) {
    if (line.line_type === "added") additions++;
    if (line.line_type === "removed") deletions++;
  }

  return { additions, deletions };
}

const statusTextClass: Record<string, string> = {
  added: "text-git-added",
  deleted: "text-git-deleted",
  modified: "text-git-modified",
  renamed: "text-git-renamed",
};

const statusBadgeClass: Record<string, string> = {
  added: "bg-git-added/12 text-git-added",
  deleted: "bg-git-deleted/12 text-git-deleted",
  modified: "bg-git-modified/12 text-git-modified",
  renamed: "bg-git-renamed/12 text-git-renamed",
};

const MAX_HUNK_ACTION_DIFF_LINES = 1200;

function parseGitHubRemoteSlug(remoteUrl: string): { owner: string; repo: string } | null {
  const normalized = remoteUrl.trim();
  const httpsMatch = normalized.match(/^https?:\/\/github\.com\/([^/]+)\/([^/]+?)(?:\.git)?\/?$/i);
  if (httpsMatch) {
    const [, owner, repo] = httpsMatch;
    return { owner, repo };
  }

  const sshMatch = normalized.match(/^git@github\.com:([^/]+)\/(.+?)(?:\.git)?$/i);
  if (sshMatch) {
    const [, owner, repo] = sshMatch;
    return { owner, repo };
  }

  return null;
}

function buildGitHubReferenceUrl(remoteUrl: string, gitRef: string): string | null {
  const slug = parseGitHubRemoteSlug(remoteUrl);
  if (!slug) return null;

  const comparisonMatch = gitRef.match(/^(.+?)(?:\.{2,3})(.+)$/);
  if (comparisonMatch) {
    const [, baseRef, targetRef] = comparisonMatch;
    return `https://github.com/${slug.owner}/${slug.repo}/compare/${encodeURIComponent(
      baseRef,
    )}...${encodeURIComponent(targetRef)}`;
  }

  return `https://github.com/${slug.owner}/${slug.repo}/commit/${encodeURIComponent(gitRef)}`;
}

function LargeDiffSectionEditor({ diff, cacheKey }: { diff: GitDiff; cacheKey: string }) {
  const { original, modified } = useMemo(() => buildMonacoDiffContent(diff), [diff]);
  const filePath = diff.new_path || diff.old_path || diff.file_path;

  return (
    <div
      className="relative overflow-hidden border-border border-t bg-background"
      style={{ height: "min(72vh, 760px)", minHeight: "420px" }}
    >
      <MonacoDiffEditorView
        originalContent={original}
        modifiedContent={modified}
        filePath={filePath}
        renderSideBySide={false}
        height="100%"
        cacheKey={`${cacheKey}_large`}
        scrollable={true}
      />
    </div>
  );
}

function EmbeddedDiffSectionEditor({
  diff,
  cacheKey,
  viewMode,
}: {
  diff: GitDiff;
  cacheKey: string;
  viewMode: "unified" | "split";
}) {
  const fontSize = useEditorSettingsStore.use.fontSize();
  const zoomLevel = useZoomStore.use.editorZoomLevel();
  const { original, modified } = useMemo(() => buildMonacoDiffContent(diff), [diff]);

  const height = useMemo(() => {
    const lineCount = Math.max(splitLines(original).length, splitLines(modified).length);
    const lineHeight = calculateLineHeight(fontSize * zoomLevel);
    return Math.max(lineCount * lineHeight + 16, 160);
  }, [original, modified, fontSize, zoomLevel]);

  const filePath = diff.new_path || diff.old_path || diff.file_path;

  return (
    <div className="border-border border-t bg-background" style={{ height: `${height}px` }}>
      <MonacoDiffEditorView
        originalContent={original}
        modifiedContent={modified}
        filePath={filePath}
        renderSideBySide={viewMode === "split"}
        height={height}
        cacheKey={cacheKey}
        scrollable={false}
      />
    </div>
  );
}

function DiffSectionEditor({
  diff,
  cacheKey,
  viewMode,
}: {
  diff: GitDiff;
  cacheKey: string;
  viewMode: "unified" | "split";
}) {
  if (shouldUseScrollableDiffEditor(diff)) {
    return <LargeDiffSectionEditor diff={diff} cacheKey={cacheKey} />;
  }

  return <EmbeddedDiffSectionEditor diff={diff} cacheKey={cacheKey} viewMode={viewMode} />;
}

const LazyDiffSectionBody = memo(function LazyDiffSectionBody({
  expanded,
  children,
}: {
  expanded: boolean;
  children: React.ReactNode;
}) {
  const bodyRef = useRef<HTMLDivElement>(null);
  const [shouldMount, setShouldMount] = useState(expanded);

  useEffect(() => {
    if (!expanded) {
      setShouldMount(false);
      return;
    }

    const element = bodyRef.current;
    if (!element) {
      setShouldMount(true);
      return;
    }

    const scrollContainer = element.closest("[data-diff-stack-scroll-container]");
    if (!(scrollContainer instanceof HTMLDivElement)) {
      setShouldMount(true);
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (entry?.isIntersecting) {
          setShouldMount(true);
          observer.disconnect();
        }
      },
      {
        root: scrollContainer,
        rootMargin: "1200px 0px",
      },
    );

    observer.observe(element);
    return () => observer.disconnect();
  }, [expanded]);

  return (
    <div
      ref={bodyRef}
      className="border-border border-t"
      style={{ contentVisibility: "auto", containIntrinsicSize: "960px" }}
    >
      {shouldMount ? children : <div className="h-[320px] bg-background" />}
    </div>
  );
});

const DiffFileSection = memo(function DiffFileSection({
  diff,
  sectionKey,
  expanded,
  onToggle,
  viewMode,
  showWhitespace,
  enableHunkActions,
  onOpenFile,
}: {
  diff: GitDiff;
  sectionKey: string;
  expanded: boolean;
  onToggle: (sectionKey: string) => void;
  onOpenFile: (filePath: string) => void | Promise<void>;
  viewMode: "unified" | "split";
  showWhitespace: boolean;
  enableHunkActions: boolean;
}) {
  const filePath = diff.new_path || diff.old_path || diff.file_path;
  const fileName = filePath.split("/").pop() || filePath;
  const directoryPath = filePath.includes("/")
    ? filePath.slice(0, filePath.lastIndexOf("/") + 1)
    : "";
  const status = getFileStatus(diff) as "added" | "deleted" | "modified" | "renamed";
  const { additions, deletions } = countStats(diff);
  const handleToggle = useCallback(() => {
    onToggle(sectionKey);
  }, [onToggle, sectionKey]);
  const handleOpenFile = useCallback(() => {
    void onOpenFile(filePath);
  }, [filePath, onOpenFile]);
  const shouldUseInlineTextDiff =
    enableHunkActions && viewMode === "unified" && diff.lines.length <= MAX_HUNK_ACTION_DIFF_LINES;

  return (
    <section className="relative isolate min-w-0 max-w-full rounded-md bg-background">
      <div className="sticky top-0 z-50 min-w-0 max-w-full bg-background">
        <div
          className={cn(
            "min-w-0 max-w-full overflow-hidden border border-border/70 bg-background shadow-[0_1px_0_rgba(0,0,0,0.04)]",
            expanded ? "rounded-t-md" : "rounded-md",
          )}
        >
          <div className="flex min-w-0 items-center">
            <button
              type="button"
              onClick={handleToggle}
              className="relative z-50 flex h-8 w-8 shrink-0 items-center justify-center text-muted-foreground hover:bg-muted/30 hover:text-foreground"
              aria-label={expanded ? "Collapse file diff" : "Expand file diff"}
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
                    "min-w-0 max-w-[45%] truncate ui-text-sm font-medium",
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
              {shouldUseInlineTextDiff ? (
                <TextDiffViewer
                  diff={diff}
                  isStaged={sectionKey.startsWith("staged:")}
                  viewMode={viewMode}
                  showWhitespace={showWhitespace}
                  isEmbeddedInScrollView={true}
                />
              ) : (
                <DiffSectionEditor diff={diff} cacheKey={sectionKey} viewMode={viewMode} />
              )}
            </LazyDiffSectionBody>
          </div>
        )
      ) : null}
    </section>
  );
});

function getInitialExpandedFiles(multiDiff: MultiFileDiff): Set<string> {
  return new Set(getInitialExpandedDiffFileKeys(multiDiff));
}

const GitDiffEditorStack = memo(function GitDiffEditorStack({
  multiDiff,
}: {
  multiDiff: MultiFileDiff;
}) {
  const workspaceStore = useWorkspaceStore();
  const buffers = useStore(workspaceStore, (s) => s.buffers);
  const activeBufferId = useStore(workspaceStore, (s) => s.paneActions.getActivePane()?.activeBufferId ?? null);
  const updateBufferContent = (bufferId: string, content: string, _markDirty: boolean, diffData?: MultiFileDiff) => {
    workspaceStore.setState((state) => ({
      ...state,
      buffers: state.buffers.map((b) =>
        b.id === bufferId && b.type === 'diff'
          ? { ...b, content, ...(diffData !== undefined ? { diffData } : {}) }
          : b,
      ),
    }));
  };
  const closeBuffer = (id: string) => workspaceStore.getState().bufferActions.closeBuffer(id);
  const rootFolderPath = useFileSystemStore((state) => state.rootFolderPath);
  const [viewMode, setViewMode] = useState<"unified" | "split">("unified");
  const [showWhitespace, setShowWhitespace] = useState(false);
  const isWorkingTree = multiDiff.commitHash === "working-tree";
  const activeBuffer = buffers.find((buffer) => buffer.id === activeBufferId) || null;
  const isWorkingTreeBuffer = activeBuffer?.path === "diff://working-tree/all-files";
  const isRefreshingRef = useRef(false);
  const handleOpenFile = useCallback(
    async (filePath: string) => {
      const repoPath = multiDiff.repoPath ?? rootFolderPath;
      const targetPath =
        filePath.startsWith("/") || filePath.startsWith("remote://")
          ? filePath
          : repoPath
            ? joinPath(repoPath, filePath)
            : filePath;

      const { handleFileSelect } = useFileSystemStore.getState();
      if (handleFileSelect) {
        handleFileSelect(targetPath, false, undefined, undefined, undefined, false);
      }
    },
    [multiDiff.repoPath, rootFolderPath],
  );
  const [githubCommitUrl, setGitHubCommitUrl] = useState<string | null>(null);
  const [expandedFiles, setExpandedFiles] = useState<Set<string>>(() =>
    getInitialExpandedFiles(multiDiff),
  );
  const handleToggleSection = useCallback((sectionKey: string) => {
    setExpandedFiles((prev) => {
      const next = new Set(prev);
      if (next.has(sectionKey)) next.delete(sectionKey);
      else next.add(sectionKey);
      return next;
    });
  }, []);

  useEffect(() => {
    const nextKeys = new Set(
      multiDiff.files.map(
        (diff, index) => multiDiff.fileKeys?.[index] ?? `${diff.file_path}:${index}`,
      ),
    );

    setExpandedFiles((previous) => {
      const nextExpanded = new Set(Array.from(previous).filter((key) => nextKeys.has(key)));

      if (nextExpanded.size === 0) {
        return getInitialExpandedFiles(multiDiff);
      }

      if (multiDiff.initiallyExpandedFileKey && nextKeys.has(multiDiff.initiallyExpandedFileKey)) {
        nextExpanded.add(multiDiff.initiallyExpandedFileKey);
      }

      return nextExpanded;
    });
  }, [multiDiff.fileKeys, multiDiff.files, multiDiff.initiallyExpandedFileKey]);

  const refreshWorkingTreeBuffer = useCallback(async () => {
    if (!isWorkingTree || !isWorkingTreeBuffer || !rootFolderPath || !activeBuffer) return;
    if (isRefreshingRef.current) return;

    isRefreshingRef.current = true;

    try {
      gitDiffCache.invalidate(rootFolderPath);
      const gitStatus = await getGitStatus(rootFolderPath);
      const nextMultiDiff = await buildWorkingTreeMultiDiff({
        repoPath: rootFolderPath,
        status: gitStatus,
        previousFileKeys: multiDiff.fileKeys,
      });

      if (nextMultiDiff.files.length === 0) {
        closeBuffer(activeBuffer.id);
        return;
      }

      updateBufferContent(activeBuffer.id, "", false, nextMultiDiff);
    } finally {
      isRefreshingRef.current = false;
    }
  }, [
    activeBuffer,
    closeBuffer,
    isWorkingTree,
    isWorkingTreeBuffer,
    multiDiff.fileKeys,
    rootFolderPath,
    updateBufferContent,
  ]);

  useEffect(() => {
    if (!isWorkingTree) return;

    const handleGitStatusChanged = () => {
      window.setTimeout(() => {
        void refreshWorkingTreeBuffer();
      }, 50);
    };

    window.addEventListener("git-status-changed", handleGitStatusChanged);
    return () => {
      window.removeEventListener("git-status-changed", handleGitStatusChanged);
    };
  }, [isWorkingTree, refreshWorkingTreeBuffer]);

  useEffect(() => {
    if (isWorkingTree || multiDiff.commitHash.startsWith("stash@{")) {
      setGitHubCommitUrl(null);
      return;
    }

    const repoPath = multiDiff.repoPath ?? rootFolderPath;
    if (!repoPath) {
      setGitHubCommitUrl(null);
      return;
    }

    let isCancelled = false;

    const loadGitHubCommitUrl = async () => {
      const remotes = await getRemotes(repoPath);
      const candidate =
        remotes.find((remote) => remote.name === "origin")?.url ?? remotes[0]?.url ?? null;
      const nextUrl = candidate ? buildGitHubReferenceUrl(candidate, multiDiff.commitHash) : null;
      if (!isCancelled) {
        setGitHubCommitUrl(nextUrl);
      }
    };

    void loadGitHubCommitUrl();

    return () => {
      isCancelled = true;
    };
  }, [isWorkingTree, multiDiff.commitHash, multiDiff.repoPath, rootFolderPath]);

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <Breadcrumb
        filePathOverride={multiDiff.title || "Uncommitted Changes"}
        interactive={false}
        showPath={false}
        showDefaultActions={true}
        extraLeftContent={
          <div className="ui-text-sm flex items-center gap-2 text-muted-foreground">
            <span>
              {multiDiff.totalFiles} changed file
              {multiDiff.totalFiles !== 1 ? "s" : ""}
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
            <Tooltip content={showWhitespace ? "Hide whitespace" : "Show whitespace"} side="bottom">
              <Button
                type="button"
                variant="ghost"
                active={showWhitespace}
                onClick={() => setShowWhitespace((prev) => !prev)}
                className={cn("h-5 gap-1 px-1.5 text-muted-foreground", showWhitespace && "text-foreground")}
                aria-label={showWhitespace ? "Hide whitespace" : "Show whitespace"}
              >
                <Trash2 />
                {showWhitespace ? <Check /> : null}
              </Button>
            </Tooltip>
            <div className="flex items-center gap-0.5">
              <Tooltip content="Unified view" side="bottom">
                <Button
                  type="button"
                  variant="ghost"
                  active={viewMode === "unified"}
                  onClick={() => setViewMode("unified")}
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
                  active={viewMode === "split"}
                  onClick={() => setViewMode("split")}
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

      {!isWorkingTree &&
      (multiDiff.commitMessage || multiDiff.commitAuthor || multiDiff.commitDate) ? (
        <div className="bg-background px-2 py-2">
          <div className="px-1 py-1.5">
            {multiDiff.commitMessage ? (
              <div className="ui-text-sm font-medium text-foreground">{multiDiff.commitMessage}</div>
            ) : null}
            {multiDiff.commitDescription ? (
              <div className="ui-text-sm mt-2 whitespace-pre-wrap text-muted-foreground">
                {multiDiff.commitDescription}
              </div>
            ) : null}
            <div className="ui-text-sm mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground">
              {multiDiff.commitAuthor ? <span>{multiDiff.commitAuthor}</span> : null}
              {multiDiff.commitDate ? (
                <span>{formatRelativeDate(multiDiff.commitDate)}</span>
              ) : null}
              <Badge size="sm" variant="secondary">
                {multiDiff.commitHash}
              </Badge>
            </div>
          </div>
        </div>
      ) : null}

      <div
        className="min-h-0 flex-1 overflow-auto px-2 pb-2"
        style={{ overflowAnchor: "none" }}
        data-diff-stack-scroll-container
      >
        <div className="flex min-w-0 max-w-full flex-col gap-2 rounded-md">
          {multiDiff.files.map((diff, index) => {
            const sectionKey = multiDiff.fileKeys?.[index] ?? `${diff.file_path}:${index}`;

            return (
              <DiffFileSection
                key={sectionKey}
                diff={diff}
                sectionKey={sectionKey}
                expanded={expandedFiles.has(sectionKey)}
                viewMode={viewMode}
                showWhitespace={showWhitespace}
                enableHunkActions={isWorkingTree}
                onToggle={handleToggleSection}
                onOpenFile={handleOpenFile}
              />
            );
          })}
        </div>
      </div>
    </div>
  );
});

export default GitDiffEditorStack;
