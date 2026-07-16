export function getFileName(path: string): string {
  return path.split('/').pop() ?? path
}
/** Athas alias */
export const getFilenameFromPath = getFileName
