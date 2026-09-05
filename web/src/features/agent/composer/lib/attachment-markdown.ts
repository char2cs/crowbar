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

export function textAttachmentMarkdown(id: string, text: string): string {
  return `\`\`\`text-attachment:${id}\n${text}\n\`\`\``
}

export function excalidrawMarkdown(id: string, sceneJson: string): string {
  return `\`\`\`excalidraw:${id}\n${sceneJson}\n\`\`\``
}

export function imageMarkdown(alt: string, ref: string): string {
  return `![${alt}](${ref})`
}

export function fileMarkdown(filename: string, ref: string): string {
  return `[${filename}](${ref})`
}
