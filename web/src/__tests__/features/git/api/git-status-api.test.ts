import { afterEach, describe, expect, it, vi } from "vitest";
import { getGitStatus } from "@/features/git/api/git-status-api";

function mockFetchEnvelope(data: unknown): void {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ success: true, data }),
    })),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("getGitStatus", () => {
  // Regression: the backend serialises a clean working tree as `files: null`.
  // Downstream code (toWorkspaceGitStatus, panels) does `status.files.length`,
  // which threw and silently killed the whole git data load — gitData stayed
  // 'idle' forever after a page reload on a clean workspace.
  it("normalises a clean tree's files:null to an empty array", async () => {
    mockFetchEnvelope({ branch: "main", ahead: 0, behind: 0, files: null });

    const status = await getGitStatus("ws-1");

    expect(status).not.toBeNull();
    expect(status?.files).toEqual([]);
  });

  it("passes through a populated files array", async () => {
    const files = [{ path: "a.ts", status: "modified", staged: false }];
    mockFetchEnvelope({ branch: "main", ahead: 1, behind: 0, files });

    const status = await getGitStatus("ws-1");

    expect(status?.files).toEqual(files);
    expect(status?.branch).toBe("main");
  });

  it("returns null when the request fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        json: async () => ({ success: false, error: "boom" }),
      })),
    );

    await expect(getGitStatus("ws-1")).resolves.toBeNull();
  });
});
