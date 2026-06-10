// Stub
export async function readFileAsText(_path: string): Promise<string> {
  return ''
}
export async function getFileExtension(path: string): Promise<string> {
  return path.split('.').pop() ?? ''
}
export function getFileName(path: string): string {
  return path.split('/').pop() ?? path
}
/** Athas alias */
export const getFilenameFromPath = getFileName
export function getParentDir(path: string): string {
  return path.split('/').slice(0, -1).join('/') || '/'
}
export function joinPath(...parts: string[]): string {
  return parts.join('/').replace(/\/+/g, '/')
}
export function isAbsolutePath(path: string): boolean {
  return path.startsWith('/')
}
