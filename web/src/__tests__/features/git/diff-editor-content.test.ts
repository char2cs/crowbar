import { describe, expect, test } from "vitest";
import type { GitDiff, GitDiffLine } from "@/features/git/types/git-types";
import { buildMonacoDiffContent } from "@/features/git/utils/diff-editor-content";

const makeDiff = (lines: Array<{ type: GitDiffLine["line_type"]; content: string }>): GitDiff => ({
  file_path: "src/foo.ts",
  is_new: false,
  is_deleted: false,
  is_renamed: false,
  lines: lines.map((l, i) => ({
    line_type: l.type,
    content: l.content,
    old_line_number: i + 1,
    new_line_number: i + 1,
  })),
});

describe("buildMonacoDiffContent", () => {
  test("context lines appear on both sides", () => {
    const diff = makeDiff([{ type: "context", content: "const x = 1" }]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("const x = 1");
    expect(modified).toBe("const x = 1");
  });

  test("removed lines appear in original only", () => {
    const diff = makeDiff([{ type: "removed", content: "old line" }]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("old line");
    expect(modified).toBe("");
  });

  test("added lines appear in modified only", () => {
    const diff = makeDiff([{ type: "added", content: "new line" }]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("");
    expect(modified).toBe("new line");
  });

  test("header lines are excluded from both sides", () => {
    const diff = makeDiff([
      { type: "header", content: "@@ -1,3 +1,3 @@" },
      { type: "context", content: "kept" },
    ]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("kept");
    expect(modified).toBe("kept");
  });

  test("mixed diff produces correct original and modified strings", () => {
    const diff = makeDiff([
      { type: "context", content: "line1" },
      { type: "removed", content: "old" },
      { type: "added", content: "new" },
      { type: "context", content: "line4" },
    ]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("line1\nold\nline4");
    expect(modified).toBe("line1\nnew\nline4");
  });

  test("empty diff produces empty strings", () => {
    const diff = makeDiff([]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("");
    expect(modified).toBe("");
  });

  test("raw_patch diff with empty lines produces empty strings", () => {
    const diff: GitDiff = {
      file_path: "src/big.ts",
      is_new: false,
      is_deleted: false,
      is_renamed: false,
      raw_patch: "--- a/src/big.ts\n+++ b/src/big.ts\n@@ -1 +1 @@\n-old\n+new",
      lines: [],
    };
    const { original, modified } = buildMonacoDiffContent(diff);
    // raw_patch is not parsed by buildMonacoDiffContent; callers must handle this case
    expect(original).toBe("");
    expect(modified).toBe("");
  });
});
