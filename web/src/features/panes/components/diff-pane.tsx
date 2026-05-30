import { lazy, Suspense } from "react";
import type { GitHunk } from "@/features/git/types/git-types";

const DiffViewer = lazy(() => import("@/features/git/components/diff/git-diff-viewer"));

interface DiffPaneProps {
  onStageHunk: (hunk: GitHunk) => Promise<void>;
  onUnstageHunk: (hunk: GitHunk) => Promise<void>;
}

export function DiffPane({ onStageHunk, onUnstageHunk }: DiffPaneProps) {
  return (
    <Suspense fallback={null}>
      <DiffViewer onStageHunk={onStageHunk} onUnstageHunk={onUnstageHunk} />
    </Suspense>
  );
}
