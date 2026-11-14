// The id suffix on fenced kinds is load-bearing: without it the renderer treats
// the fence as a code block that merely mentions the tag.

/** One backtick longer than the content's longest run, so the content can't close it. */
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

/** Keyed off the response's `contentType` — what the daemon stored, not a guess. */
export function markdownForUpload(result: {
  filename: string
  ref: string
  contentType: string
}): string {
  return result.contentType.startsWith('image/')
    ? imageMarkdown(result.filename, result.ref)
    : fileMarkdown(result.filename, result.ref)
}
