// File-reference links in terminal output: makes path-like tokens (as printed
// by compilers, test runners, Claude Code, git, ...) clickable and opens them
// inside Crowbar's editor. URLs are handled separately by the web-links addon;
// this provider deliberately skips anything that is part of a URL.
import type { IBufferLine, IDisposable, ILink, Terminal } from '@xterm/xterm'

export interface FileLinkCandidate {
  /** The full matched text, including any trailing :line[:col] suffix. */
  text: string
  /** The path portion, without the :line[:col] suffix. */
  path: string
  line?: number
  /** 0-based index of the match within the row text. */
  startIndex: number
}

// A candidate must look like a real path: either rooted (/, ./, ../, ~/) or
// containing at least one internal slash (web/src/foo.ts). A :line[:col]
// suffix is captured so "foo.go:12:3" links as a whole. Bare filenames
// without a slash are NOT matched — too many false positives (domains,
// package names) without filesystem validation.
const FILE_LINK_RE =
  /(?:^|[\s"'`([<])((?:~\/|\.{1,2}\/|\/)?[\w.@+~-]+(?:\/[\w.@+~-]+)+\/?|\/[\w.@+~-]+)((?::\d+){1,2})?/g

// Matched text whose immediate left context ends with "://" or is preceded by
// a URL scheme belongs to a URL — the web-links addon owns those.
const URL_PREFIX_RE = /(?:[a-zA-Z][a-zA-Z0-9+.-]*:\/\/|www\.)[^\s]*$/

// "." is a valid path character (extensions, dotfiles), so prose that runs
// straight into a path with no space ("...design.md. Please review") has its
// sentence-ending period swallowed into the match. Trim trailing dots the
// path can't legitimately end with.
const TRAILING_DOTS_RE = /\.+$/

export function parseFileLinkCandidates(rowText: string): FileLinkCandidate[] {
  const out: FileLinkCandidate[] = []
  FILE_LINK_RE.lastIndex = 0
  for (let m = FILE_LINK_RE.exec(rowText); m !== null; m = FILE_LINK_RE.exec(rowText)) {
    const path = m[1].replace(TRAILING_DOTS_RE, '')
    if (!path) continue
    const suffix = m[2] ?? ''
    const startIndex = m.index + m[0].indexOf(m[1])
    // Skip path-looking tails of URLs ("https://github.com/a/b").
    if (URL_PREFIX_RE.test(rowText.slice(0, startIndex))) continue
    const line = suffix ? Number(suffix.slice(1).split(':')[0]) : undefined
    out.push({ text: path + suffix, path, line, startIndex })
  }
  return out
}

// Resolves a matched path against the terminal's working root. "~" cannot be
// expanded client-side, so those candidates resolve only if the root is home
// — skip them instead of opening a wrong file.
export function resolveFileLinkPath(path: string, root: string | undefined): string | null {
  if (path.startsWith('~')) return null
  if (path.startsWith('/')) return path
  if (!root || !root.startsWith('/')) return null
  const parts: string[] = root.split('/')
  for (const seg of path.split('/')) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') {
      if (parts.length > 1) parts.pop()
      continue
    }
    parts.push(seg)
  }
  return parts.join('/')
}

// Converts an absolute path into a workspace-relative one, or null when the
// path lies outside the workspace root. The files API (and therefore
// handleFileOpen) is workspace-scoped and only accepts worktree-relative
// paths — an absolute path is rejected by the daemon's path guard.
export function workspaceRelativePath(
  absolutePath: string,
  root: string | undefined,
): string | null {
  if (!root || !root.startsWith('/') || !absolutePath.startsWith('/')) return null
  const prefix = root.endsWith('/') ? root : `${root}/`
  if (!absolutePath.startsWith(prefix)) return null
  const rel = absolutePath.slice(prefix.length)
  return rel === '' ? null : rel
}

export interface TerminalFileLinksOptions {
  /** The directory relative paths resolve against (terminal cwd / worktree). */
  getRoot: () => string | undefined
  /** Opens the resolved absolute path in Crowbar's editor. */
  openFile: (absolutePath: string, line?: number) => void
  /** Called when a clicked candidate cannot be resolved to an absolute path. */
  onUnresolved?: (candidateText: string) => void
}

// Registers an xterm link provider for file references. The provider is owned
// by the terminal instance and is disposed with it.
export function registerTerminalFileLinks(
  terminal: Terminal,
  options: TerminalFileLinksOptions,
): IDisposable {
  return terminal.registerLinkProvider({
    provideLinks(bufferLineNumber: number, callback: (links: ILink[] | undefined) => void) {
      const bufferLine: IBufferLine | undefined = terminal.buffer.active.getLine(
        bufferLineNumber - 1,
      )
      if (!bufferLine) {
        callback(undefined)
        return
      }
      const rowText = bufferLine.translateToString(true)
      const links: ILink[] = parseFileLinkCandidates(rowText).map((candidate) => ({
        text: candidate.text,
        // xterm ranges are 1-based and inclusive.
        range: {
          start: { x: candidate.startIndex + 1, y: bufferLineNumber },
          end: { x: candidate.startIndex + candidate.text.length, y: bufferLineNumber },
        },
        activate: () => {
          const resolved = resolveFileLinkPath(candidate.path, options.getRoot())
          if (resolved) options.openFile(resolved, candidate.line)
          else options.onUnresolved?.(candidate.text)
        },
      }))
      callback(links.length > 0 ? links : undefined)
    },
  })
}
