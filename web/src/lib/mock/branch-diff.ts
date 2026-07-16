import type { ReviewConversation } from '@/features/workspace/stores/slices/branch-review-slice'
import type { GitDiffLine } from '@/features/git/types/git-types'

/**
 * Generate a realistic block of TypeScript/Go diff lines for large files.
 * Produces `count` lines alternating between added/context/removed groups.
 */
export function generateLargeFileDiff(_filePath: string, count: number): GitDiffLine[] {
  const lines: GitDiffLine[] = []

  const tsSnippets = [
    "import { useCallback, useEffect, useRef, useState } from 'react'",
    "import type { QueryClient } from '@tanstack/react-query'",
    "import { create } from 'zustand'",
    "import { immer } from 'zustand/middleware/immer'",
    '',
    'export interface QueryLayerConfig {',
    '  baseUrl: string',
    '  timeout: number',
    '  retries: number',
    '  cacheTime: number',
    '  staleTime: number',
    '}',
    '',
    'const DEFAULT_CONFIG: QueryLayerConfig = {',
    "  baseUrl: '/api/v2',",
    '  timeout: 30_000,',
    '  retries: 3,',
    '  cacheTime: 5 * 60 * 1000,',
    '  staleTime: 60_000,',
    '}',
    '',
    'export class QueryLayer {',
    '  private config: QueryLayerConfig',
    '  private client: QueryClient',
    '  private abortControllers = new Map<string, AbortController>()',
    '',
    '  constructor(config: Partial<QueryLayerConfig> = {}) {',
    '    this.config = { ...DEFAULT_CONFIG, ...config }',
    '    this.client = new QueryClient({',
    '      defaultOptions: {',
    '        queries: {',
    '          staleTime: this.config.staleTime,',
    '          gcTime: this.config.cacheTime,',
    '          retry: this.config.retries,',
    '        },',
    '      },',
    '    })',
    '  }',
    '',
    '  async fetch<T>(key: string, fetcher: () => Promise<T>): Promise<T> {',
    '    const controller = new AbortController()',
    '    this.abortControllers.set(key, controller)',
    '    try {',
    '      const signal = controller.signal',
    '      const result = await Promise.race([',
    '        fetcher(),',
    '        new Promise<never>((_, reject) =>',
    '          setTimeout(() => reject(new Error("Timeout")), this.config.timeout)',
    '        ),',
    '      ])',
    '      return result',
    '    } finally {',
    '      this.abortControllers.delete(key)',
    '    }',
    '  }',
    '',
    '  invalidate(key: string): void {',
    '    this.client.invalidateQueries({ queryKey: [key] })',
    '  }',
    '',
    '  abort(key: string): void {',
    '    const ctrl = this.abortControllers.get(key)',
    '    if (ctrl) { ctrl.abort(); this.abortControllers.delete(key) }',
    '  }',
    '',
    '  abortAll(): void {',
    '    this.abortControllers.forEach(ctrl => ctrl.abort())',
    '    this.abortControllers.clear()',
    '  }',
    '',
    '  destroy(): void {',
    '    this.abortAll()',
    '    this.client.clear()',
    '  }',
    '}',
    '',
    'export function createQueryHook<T>(',
    '  key: string,',
    '  fetcher: () => Promise<T>,',
    '  options?: { staleTime?: number; enabled?: boolean },',
    ') {',
    '  return function useQueryHook() {',
    '    const [data, setData] = useState<T | undefined>(undefined)',
    '    const [error, setError] = useState<Error | null>(null)',
    "    const [status, setStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')",
    '    const layerRef = useRef<QueryLayer | null>(null)',
    '',
    '    useEffect(() => {',
    '      if (options?.enabled === false) return',
    "      setStatus('loading')",
    '      const layer = layerRef.current ?? new QueryLayer()',
    '      layerRef.current = layer',
    '      layer.fetch(key, fetcher)',
    "        .then(d => { setData(d); setStatus('success') })",
    "        .catch(e => { setError(e); setStatus('error') })",
    '      return () => { layer.abort(key) }',
    '    }, [options?.enabled])',
    '',
    '    return { data, error, status }',
    '  }',
    '}',
  ]

  const removedSnippets = [
    'export async function legacyFetch(url: string) {',
    '  const res = await fetch(url)',
    '  if (!res.ok) throw new Error(`HTTP ${res.status}`)',
    '  return res.json()',
    '}',
    '',
    'export const globalCache = new Map<string, unknown>()',
    '',
    'export function clearCache() { globalCache.clear() }',
    '',
    'export function getCachedOrFetch<T>(key: string, fetcher: () => Promise<T>) {',
    '  const cached = globalCache.get(key)',
    '  if (cached !== undefined) return Promise.resolve(cached as T)',
    '  return fetcher().then(d => { globalCache.set(key, d); return d })',
    '}',
  ]

  // header
  const totalAdded = Math.ceil(count * 0.65)
  const totalRemoved = count - totalAdded
  lines.push({ line_type: 'header', content: `@@ -1,${totalRemoved} +1,${totalAdded} @@` })

  let oldLine = 1
  let newLine = 1
  let snippetIdx = 0
  let removedIdx = 0
  let generated = 0

  while (generated < count) {
    // emit a block of removed lines
    const removedBlock = Math.min(3, totalRemoved - removedIdx, count - generated)
    for (let i = 0; i < removedBlock; i++) {
      lines.push({
        line_type: 'removed',
        content: removedSnippets[removedIdx % removedSnippets.length],
        old_line_number: oldLine++,
      })
      removedIdx++
      generated++
    }

    // emit a block of added lines
    const addedBlock = Math.min(5, count - generated)
    for (let i = 0; i < addedBlock; i++) {
      lines.push({
        line_type: 'added',
        content: tsSnippets[snippetIdx % tsSnippets.length],
        new_line_number: newLine++,
      })
      snippetIdx++
      generated++
    }

    // emit a context separator
    if (generated < count - 2) {
      lines.push({
        line_type: 'context',
        content: '',
        old_line_number: oldLine++,
        new_line_number: newLine++,
      })
      generated++

      // new hunk header
      if (generated % 40 === 0 && generated < count - 5) {
        lines.push({
          line_type: 'header',
          content: `@@ -${oldLine},${Math.min(20, totalRemoved - removedIdx)} +${newLine},${Math.min(30, totalAdded - (newLine - 1))} @@`,
        })
        generated++
      }
    }
  }

  return lines
}

// ---------------------------------------------------------------------------
// Workspace-specific conversations
// ---------------------------------------------------------------------------

export type BranchReviewChat = ReviewConversation
