// Stub
import type { AppFile } from "@/features/file-system/types/app"

export function buildFileTree(_paths: string[]): unknown[] { return [] }
export function sortFileTree<T>(nodes: T[]): T[] { return nodes }
export function findFileInTree(_tree: AppFile[], _path: string): AppFile | null { return null }
export function flattenFileTree(_tree: AppFile[]): AppFile[] { return [] }
