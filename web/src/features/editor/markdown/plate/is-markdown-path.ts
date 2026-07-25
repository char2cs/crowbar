/** True for files the Plate rich editor owns. `.mdx` is deliberately excluded —
 *  MDX (JSX in markdown) does not round-trip through a markdown serializer, so
 *  those files stay on Monaco (see design spec, Risks). */
export function isMarkdownPath(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase()
  return ext === 'md' || ext === 'markdown'
}
