import { lazy, Suspense, useCallback, useRef, useState } from "react";
import type { GitHunk } from "@/features/git/types/git-types";
import { CommentComposer } from "./comment-composer";
import { ReviewThreadView } from "./review-thread";

const DiffViewer = lazy(() => import("@/features/git/components/diff/git-diff-viewer"));

interface DiffPaneProps {
  onStageHunk: (hunk: GitHunk) => Promise<void>;
  onUnstageHunk: (hunk: GitHunk) => Promise<void>;
}

interface ReviewMessage {
  id: string;
  author: string | null;
  isAgent: boolean;
  body: string;
  createdAt: string;
}

interface ReviewThread {
  id: string;
  filePath: string;
  lineNumber: number;
  side: "left" | "right";
  messages: ReviewMessage[];
  isResolved: boolean;
}

interface PendingComposer {
  lineNumber: number;
  side: "left" | "right";
  anchorY: number;
}

function estimateLineFromClickY(
  containerEl: HTMLElement,
  clickY: number,
): number {
  const rect = containerEl.getBoundingClientRect();
  const relativeY = clickY - rect.top;
  // Estimate ~20px per line (approximate default line height).
  const estimated = Math.max(1, Math.round(relativeY / 20));
  return estimated;
}

function detectSideFromTarget(target: EventTarget | null): "left" | "right" {
  if (!(target instanceof Element)) return "right";
  // In split view the left panel has border-r; check if element is inside the
  // first of two grid columns.
  const leftPanel = target.closest(".grid-cols-2 > :first-child");
  if (leftPanel) return "left";
  return "right";
}

export function DiffPane({ onStageHunk, onUnstageHunk }: DiffPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [threads, setThreads] = useState<ReviewThread[]>([]);
  const [pending, setPending] = useState<PendingComposer | null>(null);

  const handleContainerClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      // Only trigger when Alt/Option key is held — avoids interfering with
      // normal diff interaction (scrolling, code navigation, hunk actions).
      if (!e.altKey) return;

      e.preventDefault();
      e.stopPropagation();

      const container = containerRef.current;
      if (!container) return;

      const line = estimateLineFromClickY(container, e.clientY);
      const side = detectSideFromTarget(e.target);

      setPending({ lineNumber: line, side, anchorY: e.clientY });
    },
    [],
  );

  const handleComposerSubmit = useCallback(
    (body: string) => {
      if (!pending) return;

      const newThread: ReviewThread = {
        id: crypto.randomUUID(),
        filePath: "",
        lineNumber: pending.lineNumber,
        side: pending.side,
        messages: [
          {
            id: crypto.randomUUID(),
            author: null,
            isAgent: false,
            body,
            createdAt: new Date().toISOString(),
          },
        ],
        isResolved: false,
      };

      setThreads((prev) => [...prev, newThread]);
      setPending(null);
    },
    [pending],
  );

  const handleComposerCancel = useCallback(() => {
    setPending(null);
  }, []);

  const handleThreadReply = useCallback((threadId: string, body: string) => {
    setThreads((prev) =>
      prev.map((t) =>
        t.id === threadId
          ? {
              ...t,
              messages: [
                ...t.messages,
                {
                  id: crypto.randomUUID(),
                  author: null,
                  isAgent: false,
                  body,
                  createdAt: new Date().toISOString(),
                },
              ],
            }
          : t,
      ),
    );
  }, []);

  const handleThreadResolve = useCallback((threadId: string) => {
    setThreads((prev) =>
      prev.map((t) =>
        t.id === threadId ? { ...t, isResolved: true } : t,
      ),
    );
  }, []);

  const handleThreadDelete = useCallback((threadId: string) => {
    setThreads((prev) => prev.filter((t) => t.id !== threadId));
  }, []);

  const hasThreadsOrPending = threads.length > 0 || pending !== null;

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Diff viewer — click with Alt/Option held to open a comment composer */}
      <div
        ref={containerRef}
        className="relative min-h-0 flex-1"
        onClick={handleContainerClick}
      >
        <Suspense fallback={null}>
          <DiffViewer onStageHunk={onStageHunk} onUnstageHunk={onUnstageHunk} />
        </Suspense>

        {/* Hint badge — only shown when no threads/pending exist yet */}
        {!hasThreadsOrPending && (
          <div
            className="pointer-events-none absolute bottom-3 right-3 z-10 rounded-md border border-border/40 bg-background/80 px-2 py-1 text-[10px] text-muted-foreground/50 backdrop-blur-sm"
            aria-hidden="true"
          >
            Alt-click to comment
          </div>
        )}
      </div>

      {/* Inline comment panel — shown below the diff when there is activity */}
      {hasThreadsOrPending && (
        <div className="flex max-h-[40%] min-h-0 flex-col gap-1 overflow-y-auto border-t border-border bg-background px-3 py-2">
          {threads.map((thread) => (
            <ReviewThreadView
              key={thread.id}
              thread={thread}
              onReply={(body) => handleThreadReply(thread.id, body)}
              onResolve={() => handleThreadResolve(thread.id)}
              onDelete={() => handleThreadDelete(thread.id)}
            />
          ))}

          {pending && (
            <CommentComposer
              title={`Add a comment on line ${pending.side === "left" ? "L" : "R"}${pending.lineNumber}`}
              onSubmit={handleComposerSubmit}
              onCancel={handleComposerCancel}
            />
          )}
        </div>
      )}
    </div>
  );
}
