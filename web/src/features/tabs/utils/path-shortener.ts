import type { PaneContent } from '@/features/panes/types/pane-content'
import { isVirtualContent } from '@/features/panes/types/pane-content'

/**
 * Get path segments (directories) from a file path
 */
function getPathSegments(filePath: string): string[] {
  // Normalize path separators to forward slash
  const normalized = filePath.replace(/\\/g, '/')
  const parts = normalized.split('/')
  // Return all parts except the last one (filename)
  return parts.slice(0, -1)
}

/**
 * Get the filename from a path
 */
function getFileName(filePath: string): string {
  const normalized = filePath.replace(/\\/g, '/')
  const parts = normalized.split('/')
  return parts[parts.length - 1] || ''
}

/** The shortest `../dir/name` tail of each distinct path that tells it from the others. */
function distinctPathLabels(paths: string[], fileName: string): Map<string, string> {
  const segmentsOf = new Map(paths.map((path) => [path, getPathSegments(path)]))
  const label = (segments: string[]) =>
    segments.length > 0 ? `../${segments.join('/')}/${fileName}` : fileName
  if (paths.length === 1) return new Map([[paths[0], fileName]])

  const deepest = Math.max(...[...segmentsOf.values()].map((segments) => segments.length))
  for (let needed = 1; needed <= deepest; needed++) {
    const tails = new Set([...segmentsOf.values()].map((s) => s.slice(-needed).join('/')))
    if (tails.size === paths.length) {
      return new Map(paths.map((path) => [path, label(segmentsOf.get(path)!.slice(-needed))]))
    }
  }
  return new Map(paths.map((path) => [path, label(segmentsOf.get(path)!)]))
}

/**
 * Calculate minimal distinguishing display names for buffers: same-named files
 * by the shortest differing directory tail, and one path open from two
 * workspaces by the workspace's name, as restore failures name them.
 * Returns a map of buffer ID to display name
 */
export function calculateDisplayNames(
  buffers: PaneContent[],
  workspaceNames: ReadonlyMap<string, string>,
): Map<string, string> {
  const displayNames = new Map<string, string>()

  // Skip virtual or special buffers
  const regularBuffers = buffers.filter(
    (b) => !isVirtualContent(b) && b.path !== 'extensions://marketplace',
  )

  // Group buffers by filename
  const fileNameGroups = new Map<string, PaneContent[]>()
  for (const buffer of regularBuffers) {
    // Non-virtual, non-marketplace buffers always carry a real path (see
    // OpenEditorTabSpec) — the `?? ''` only guards the type.
    const fileName = getFileName(buffer.path ?? '')
    if (!fileNameGroups.has(fileName)) {
      fileNameGroups.set(fileName, [])
    }
    fileNameGroups.get(fileName)!.push(buffer)
  }

  for (const [fileName, group] of fileNameGroups) {
    const paths = [...new Set(group.map((b) => b.path ?? ''))]
    const labels = distinctPathLabels(paths, fileName)
    for (const buffer of group) {
      const label = labels.get(buffer.path ?? '')!
      const twin = group.some(
        (other) => other.path === buffer.path && other.workspaceId !== buffer.workspaceId,
      )
      displayNames.set(
        buffer.id,
        twin ? `${label} (${workspaceNames.get(buffer.workspaceId) ?? buffer.workspaceId})` : label,
      )
    }
  }

  // Set display names for special/virtual buffers
  for (const buffer of buffers) {
    if (!displayNames.has(buffer.id)) {
      displayNames.set(buffer.id, buffer.name)
    }
  }

  return displayNames
}
