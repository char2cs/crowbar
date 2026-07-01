/**
 * Whether this editor surface must NOT participate in the LSP document
 * lifecycle (didOpen / didChange / didClose).
 *
 * Diff / virtual / read-only surfaces serialize diff text (+/- lines or split
 * halves) into a buffer whose `path` can be the REAL workspace file path (the
 * git diff viewer passes `pathOverride: sourcePath`). Opening such a buffer must
 * never POST didOpen/didChange for that path: `documentChange` is not refcounted,
 * so the last (diff-garbled) change would clobber the LSP's in-memory copy of a
 * file the user may be actively editing in a real pane — corrupting diagnostics,
 * completions, hover and go-to-definition until the next genuine edit resyncs.
 *
 * Kept in its own Monaco-free module so it can be unit-tested without dragging
 * the whole monaco-editor module graph into the test environment.
 */
export function shouldSkipLsp(
  readOnly: boolean,
  isPreviewMode: boolean,
  isVirtual: boolean,
): boolean {
  return readOnly || isPreviewMode || isVirtual
}
