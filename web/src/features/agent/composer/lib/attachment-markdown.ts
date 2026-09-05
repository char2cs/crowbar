/**
 * Markdown-string encodings for the four attachment kinds. Every editor-UX
 * insertion path (paste, drop, Attach File, Excalidraw save) builds one of
 * these and deserializes it through `chatMarkdownToValue` rather than
 * constructing Slate nodes directly — see chat-markdown-editor.tsx's
 * `insertAttachmentMarkdown`.
 *
 * The id suffix on the fenced kinds is load-bearing, not decorative: the
 * rendering-side plugin requires it before treating a `text-attachment`/
 * `excalidraw` fence as a real attachment rather than a code block that
 * merely mentions the tag (see the design spec's "fence-tag collision" test).
 */

/**
 * Compute a markdown fence that will safely wrap content without being
 * prematurely closed by backtick sequences within the content.
 *
 * CommonMark rule: a line containing only backticks, whose count is >= the
 * opening fence's count, closes the fence. To be safe, we use a fence length
 * that is one more than the longest run of consecutive backticks in the content,
 * with a minimum of 3 (standard markdown fence length).
 */
function fenceFor(content: string): string {
  const runs = content.match(/`+/g) ?? []
  const longest = runs.reduce((max, run) => Math.max(max, run.length), 0)
  return '`'.repeat(Math.max(3, longest + 1))
}

export function textAttachmentMarkdown(id: string, text: string): string {
  const fence = fenceFor(text)
  return `${fence}text-attachment:${id}\n${text}\n${fence}`
}

export function excalidrawMarkdown(id: string, sceneJson: string): string {
  const fence = fenceFor(sceneJson)
  return `${fence}excalidraw:${id}\n${sceneJson}\n${fence}`
}

export function imageMarkdown(alt: string, ref: string): string {
  return `![${alt}](${ref})`
}

export function fileMarkdown(filename: string, ref: string): string {
  return `[${filename}](${ref})`
}
