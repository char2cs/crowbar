export function getFileName(path: string): string {
  return path.split('/').pop() ?? path
}
/** Alias. */
export const getFilenameFromPath = getFileName
