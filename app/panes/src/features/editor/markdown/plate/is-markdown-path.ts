export function isMarkdownPath(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase()
  return ext === 'md' || ext === 'markdown'
}
