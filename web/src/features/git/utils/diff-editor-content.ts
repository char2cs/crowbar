import type { TokenEntry } from "@/features/panes/types/pane-content";
import type { MultiFileDiff } from "../types/git-diff-types";
import type { GitDiff, GitDiffLine } from "../types/git-types";

const DIFF_ACCORDION_PREFIX = "\uE000ATHAS_DIFF_FILE ";

export interface DiffAccordionLineMeta {
  name: string;
  path: string;
  status: "added" | "deleted" | "modified" | "renamed";
  collapsed: boolean;
  hiddenCount?: number;
}

function getDisplayPath(diff: GitDiff): string {
  return diff.new_path || diff.old_path || diff.file_path;
}

function getFileStatus(diff: GitDiff): DiffAccordionLineMeta["status"] {
  if (diff.is_new) return "added";
  if (diff.is_deleted) return "deleted";
  if (diff.is_renamed) return "renamed";
  return "modified";
}

function createDiffAccordionLine(diff: GitDiff): string {
  const path = getDisplayPath(diff);
  const name = path.split("/").pop() || path;
  return `${DIFF_ACCORDION_PREFIX}${JSON.stringify({
    name,
    path,
    status: getFileStatus(diff),
    collapsed: false,
  } satisfies DiffAccordionLineMeta)}`;
}

function toDiffLineText(line: GitDiffLine): string {
  switch (line.line_type) {
    case "header":
      return line.content;
    case "added":
      return `+${line.content}`;
    case "removed":
      return `-${line.content}`;
    case "context":
    default:
      return ` ${line.content}`;
  }
}

function serializeFileHeader(diff: GitDiff): string[] {
  const displayPath = getDisplayPath(diff);
  const oldPath = diff.old_path || displayPath;
  const newPath = diff.new_path || displayPath;

  return [
    `diff --git a/${oldPath} b/${newPath}`,
    `--- ${diff.is_new ? "/dev/null" : `a/${oldPath}`}`,
    `+++ ${diff.is_deleted ? "/dev/null" : `b/${newPath}`}`,
  ];
}

export function serializeGitDiffForEditor(diff: GitDiff): string {
  if (diff.raw_patch) {
    return diff.raw_patch;
  }

  const serializedLines = diff.lines.map(toDiffLineText);
  const hasPatchHeader = serializedLines.some(
    (line) =>
      line.startsWith("diff --git ") ||
      line.startsWith("--- ") ||
      line.startsWith("+++ ") ||
      line.startsWith("index "),
  );

  return [...(hasPatchHeader ? [] : serializeFileHeader(diff)), ...serializedLines].join("\n");
}


export function serializeMultiFileDiffForEditor(multiDiff: MultiFileDiff): string {
  return multiDiff.files
    .map((diff) => [createDiffAccordionLine(diff), serializeGitDiffForEditor(diff)].join("\n"))
    .join("\n\n");
}

export function getDiffEditorPath(sourcePath: string | undefined, cacheKey: string): string {
  const rawFileName = sourcePath?.split("/").pop() || "diff";
  const fileName = `${rawFileName}.diff`;
  return `diff-editor://${cacheKey}/${fileName}`;
}

export function createDiffTokensForEditorContent(content: string): TokenEntry[] {
  const tokens: TokenEntry[] = [];
  let offset = 0;

  for (const line of content.split("\n")) {
    const lineStart = offset;
    const lineEnd = lineStart + line.length;

    const pushToken = (start: number, end: number, className: string) => {
      if (start >= end) return;
      tokens.push({
        start,
        end,
        class_name: className,
        token_type: className,
      });
    };

    if (isDiffAccordionLine(line)) {
      offset = lineEnd + 1;
      continue;
    }

    if (
      line.startsWith("diff --git") ||
      line.startsWith("index ") ||
      line.startsWith("Binary files")
    ) {
      pushToken(lineStart, lineEnd, "keyword");
    } else if (line.startsWith("@@")) {
      pushToken(lineStart, lineEnd, "attribute");
    } else if (line.startsWith("+++ ")) {
      pushToken(lineStart, lineEnd, "string");
    } else if (line.startsWith("--- ")) {
      pushToken(lineStart, lineEnd, "variable");
    } else if (line.startsWith("+")) {
      pushToken(lineStart, lineEnd, "string");
    } else if (line.startsWith("-")) {
      pushToken(lineStart, lineEnd, "variable");
    }

    offset = lineEnd + 1;
  }

  return tokens;
}

export function isDiffAccordionLine(line: string): boolean {
  return line.startsWith(DIFF_ACCORDION_PREFIX);
}

export function parseDiffAccordionLine(line: string): DiffAccordionLineMeta | null {
  if (!isDiffAccordionLine(line)) return null;

  try {
    return JSON.parse(line.slice(DIFF_ACCORDION_PREFIX.length)) as DiffAccordionLineMeta;
  } catch {
    return null;
  }
}

export function createCollapsedDiffAccordionLine(
  meta: Omit<DiffAccordionLineMeta, "collapsed">,
): string {
  return `${DIFF_ACCORDION_PREFIX}${JSON.stringify({
    ...meta,
    collapsed: true,
  } satisfies DiffAccordionLineMeta)}`;
}

export function buildMonacoDiffContent(diff: GitDiff): { original: string; modified: string } {
  const originalLines: string[] = [];
  const modifiedLines: string[] = [];

  for (const line of diff.lines) {
    if (line.line_type === "header") continue;
    if (line.line_type === "context") {
      originalLines.push(line.content);
      modifiedLines.push(line.content);
    } else if (line.line_type === "removed") {
      originalLines.push(line.content);
    } else if (line.line_type === "added") {
      modifiedLines.push(line.content);
    }
  }

  return {
    original: originalLines.join("\n"),
    modified: modifiedLines.join("\n"),
  };
}
