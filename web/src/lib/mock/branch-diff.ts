import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread, ReviewMessage, ReviewConversation } from '@/features/branch-review/types/review-types'
import type { GitDiffLine } from '@/features/git/types/git-types'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function msg(
  id: string,
  author: string | null,
  isAgent: boolean,
  body: string,
  createdAt: string,
): ReviewMessage {
  return { id, author, isAgent, body, createdAt }
}

function thread(
  id: string,
  filePath: string,
  lineNumber: number,
  side: 'left' | 'right',
  messages: ReviewMessage[],
  isResolved: boolean,
): ReviewThread {
  return { id, filePath, lineNumber, side, messages, isResolved }
}

/**
 * Generate a realistic block of TypeScript/Go diff lines for large files.
 * Produces `count` lines alternating between added/context/removed groups.
 */
function generateLargeFileDiff(_filePath: string, count: number): GitDiffLine[] {
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
    "    const [data, setData] = useState<T | undefined>(undefined)",
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
// Workspace diff data
// ---------------------------------------------------------------------------

const DIFFS: Record<string, MultiFileDiff> = {
  // ws3 — feature/app-design — medium PR, 4 TypeScript files
  ws3: {
    commitHash: 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2',
    commitMessage: 'feat(app-design): rework design token system and theme provider',
    files: [
      {
        file_path: 'src/features/settings/lib/design-tokens.ts',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 48,
        deletions: 0,
        lines: [
          { line_type: 'header', content: '@@ -0,0 +1,48 @@' },
          { line_type: 'added', content: "export type ColorScale = 50 | 100 | 200 | 300 | 400 | 500 | 600 | 700 | 800 | 900 | 950", new_line_number: 1 },
          { line_type: 'added', content: '', new_line_number: 2 },
          { line_type: 'added', content: 'export interface DesignToken {', new_line_number: 3 },
          { line_type: 'added', content: '  name: string', new_line_number: 4 },
          { line_type: 'added', content: '  value: string', new_line_number: 5 },
          { line_type: 'added', content: '  category: "color" | "spacing" | "typography" | "radius" | "shadow"', new_line_number: 6 },
          { line_type: 'added', content: '}', new_line_number: 7 },
          { line_type: 'added', content: '', new_line_number: 8 },
          { line_type: 'added', content: 'export const colorTokens: DesignToken[] = [', new_line_number: 9 },
          { line_type: 'added', content: '  { name: "--color-background", value: "hsl(0 0% 100%)", category: "color" },', new_line_number: 10 },
          { line_type: 'added', content: '  { name: "--color-foreground", value: "hsl(222 47% 11%)", category: "color" },', new_line_number: 11 },
          { line_type: 'added', content: '  { name: "--color-primary", value: "hsl(221 83% 53%)", category: "color" },', new_line_number: 12 },
          { line_type: 'added', content: '  { name: "--color-primary-foreground", value: "hsl(0 0% 100%)", category: "color" },', new_line_number: 13 },
          { line_type: 'added', content: '  { name: "--color-muted", value: "hsl(210 40% 96%)", category: "color" },', new_line_number: 14 },
          { line_type: 'added', content: '  { name: "--color-muted-foreground", value: "hsl(215 16% 47%)", category: "color" },', new_line_number: 15 },
          { line_type: 'added', content: '  { name: "--color-border", value: "hsl(214 32% 91%)", category: "color" },', new_line_number: 16 },
          { line_type: 'added', content: ']', new_line_number: 17 },
          { line_type: 'added', content: '', new_line_number: 18 },
          { line_type: 'added', content: 'export function cssVariableString(tokens: DesignToken[]): string {', new_line_number: 19 },
          { line_type: 'added', content: '  return tokens.map(t => `  ${t.name}: ${t.value};`).join("\\n")', new_line_number: 20 },
          { line_type: 'added', content: '}', new_line_number: 21 },
        ],
      },
      {
        file_path: 'src/features/settings/components/theme-provider.tsx',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 32,
        deletions: 14,
        lines: [
          { line_type: 'header', content: '@@ -1,20 +1,38 @@' },
          { line_type: 'context', content: "import { createContext, useContext, useEffect, useState } from 'react'", old_line_number: 1, new_line_number: 1 },
          { line_type: 'removed', content: "type Theme = 'dark' | 'light' | 'system'", old_line_number: 2 },
          { line_type: 'added', content: "import { colorTokens, cssVariableString } from '@/features/settings/lib/design-tokens'", new_line_number: 2 },
          { line_type: 'added', content: '', new_line_number: 3 },
          { line_type: 'added', content: "export type Theme = 'dark' | 'light' | 'system'", new_line_number: 4 },
          { line_type: 'added', content: "export type ColorScheme = 'default' | 'ocean' | 'forest' | 'rose'", new_line_number: 5 },
          { line_type: 'context', content: '', old_line_number: 3, new_line_number: 6 },
          { line_type: 'removed', content: 'interface ThemeProviderProps {', old_line_number: 4 },
          { line_type: 'removed', content: '  children: React.ReactNode', old_line_number: 5 },
          { line_type: 'removed', content: "  defaultTheme?: Theme", old_line_number: 6 },
          { line_type: 'removed', content: '  storageKey?: string', old_line_number: 7 },
          { line_type: 'removed', content: '}', old_line_number: 8 },
          { line_type: 'added', content: 'interface ThemeProviderProps {', new_line_number: 7 },
          { line_type: 'added', content: '  children: React.ReactNode', new_line_number: 8 },
          { line_type: 'added', content: '  defaultTheme?: Theme', new_line_number: 9 },
          { line_type: 'added', content: '  defaultColorScheme?: ColorScheme', new_line_number: 10 },
          { line_type: 'added', content: '  storageKey?: string', new_line_number: 11 },
          { line_type: 'added', content: '}', new_line_number: 12 },
          { line_type: 'context', content: '', old_line_number: 9, new_line_number: 13 },
          { line_type: 'context', content: 'export function ThemeProvider({', old_line_number: 10, new_line_number: 14 },
          { line_type: 'context', content: '  children,', old_line_number: 11, new_line_number: 15 },
          { line_type: 'removed', content: "  defaultTheme = 'system',", old_line_number: 12 },
          { line_type: 'removed', content: "  storageKey = 'crowbar-theme',", old_line_number: 13 },
          { line_type: 'added', content: "  defaultTheme = 'system',", new_line_number: 16 },
          { line_type: 'added', content: "  defaultColorScheme = 'default',", new_line_number: 17 },
          { line_type: 'added', content: "  storageKey = 'crowbar-theme',", new_line_number: 18 },
          { line_type: 'context', content: '}: ThemeProviderProps) {', old_line_number: 14, new_line_number: 19 },
          { line_type: 'added', content: "  const [colorScheme, setColorScheme] = useState<ColorScheme>(defaultColorScheme)", new_line_number: 20 },
          { line_type: 'added', content: '', new_line_number: 21 },
          { line_type: 'added', content: '  useEffect(() => {', new_line_number: 22 },
          { line_type: 'added', content: '    const root = window.document.documentElement', new_line_number: 23 },
          { line_type: 'added', content: "    root.setAttribute('data-color-scheme', colorScheme)", new_line_number: 24 },
          { line_type: 'added', content: "    const style = document.getElementById('dynamic-tokens') ?? document.createElement('style')", new_line_number: 25 },
          { line_type: 'added', content: "    style.id = 'dynamic-tokens'", new_line_number: 26 },
          { line_type: 'added', content: '    style.textContent = `:root {\\n${cssVariableString(colorTokens)}\\n}`', new_line_number: 27 },
          { line_type: 'added', content: '    document.head.appendChild(style)', new_line_number: 28 },
          { line_type: 'added', content: '  }, [colorScheme])', new_line_number: 29 },
        ],
      },
      {
        file_path: 'src/features/settings/components/settings-panel.tsx',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 18,
        deletions: 6,
        lines: [
          { line_type: 'header', content: '@@ -12,18 +12,30 @@' },
          { line_type: 'context', content: 'function AppearanceSection() {', old_line_number: 12, new_line_number: 12 },
          { line_type: 'context', content: '  const theme = useSettingsStore(s => s.theme)', old_line_number: 13, new_line_number: 13 },
          { line_type: 'removed', content: '  const setTheme = useSettingsStore(s => s.setTheme)', old_line_number: 14 },
          { line_type: 'added', content: '  const colorScheme = useSettingsStore(s => s.colorScheme)', new_line_number: 14 },
          { line_type: 'added', content: '  const setTheme = useSettingsStore(s => s.setTheme)', new_line_number: 15 },
          { line_type: 'added', content: '  const setColorScheme = useSettingsStore(s => s.setColorScheme)', new_line_number: 16 },
          { line_type: 'context', content: '', old_line_number: 15, new_line_number: 17 },
          { line_type: 'context', content: '  return (', old_line_number: 16, new_line_number: 18 },
          { line_type: 'removed', content: '    <div className="space-y-4">', old_line_number: 17 },
          { line_type: 'added', content: '    <div className="space-y-6">', new_line_number: 19 },
          { line_type: 'removed', content: '      <ThemeToggle value={theme} onChange={setTheme} />', old_line_number: 18 },
          { line_type: 'added', content: '      <ThemeToggle value={theme} onChange={setTheme} />', new_line_number: 20 },
          { line_type: 'added', content: '      <ColorSchemeSelector value={colorScheme} onChange={setColorScheme} />', new_line_number: 21 },
          { line_type: 'context', content: '    </div>', old_line_number: 19, new_line_number: 22 },
          { line_type: 'context', content: '  )', old_line_number: 20, new_line_number: 23 },
          { line_type: 'context', content: '}', old_line_number: 21, new_line_number: 24 },
        ],
      },
      {
        file_path: 'src/features/settings/lib/settings-store.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 12,
        deletions: 3,
        lines: [
          { line_type: 'header', content: '@@ -18,9 +18,18 @@' },
          { line_type: 'context', content: 'interface SettingsState {', old_line_number: 18, new_line_number: 18 },
          { line_type: 'context', content: '  theme: Theme', old_line_number: 19, new_line_number: 19 },
          { line_type: 'removed', content: '  setTheme: (theme: Theme) => void', old_line_number: 20 },
          { line_type: 'added', content: '  colorScheme: ColorScheme', new_line_number: 20 },
          { line_type: 'added', content: '  setTheme: (theme: Theme) => void', new_line_number: 21 },
          { line_type: 'added', content: '  setColorScheme: (scheme: ColorScheme) => void', new_line_number: 22 },
          { line_type: 'context', content: '}', old_line_number: 21, new_line_number: 23 },
          { line_type: 'context', content: '', old_line_number: 22, new_line_number: 24 },
          { line_type: 'removed', content: "export const useSettingsStore = create<SettingsState>()(set => ({", old_line_number: 23 },
          { line_type: 'removed', content: "  theme: 'system',", old_line_number: 24 },
          { line_type: 'added', content: "export const useSettingsStore = create<SettingsState>()(immer(set => ({", new_line_number: 25 },
          { line_type: 'added', content: "  theme: 'system',", new_line_number: 26 },
          { line_type: 'added', content: "  colorScheme: 'default',", new_line_number: 27 },
          { line_type: 'added', content: '  setTheme: (theme) => set(s => { s.theme = theme }),', new_line_number: 28 },
          { line_type: 'added', content: '  setColorScheme: (scheme) => set(s => { s.colorScheme = scheme }),', new_line_number: 29 },
          { line_type: 'added', content: '})))', new_line_number: 30 },
        ],
      },
    ],
    totalFiles: 4,
    totalAdditions: 110,
    totalDeletions: 23,
  },

  // ws1 — enhancement/scaffold — small new feature (parent: ws3)
  ws1: {
    commitHash: 'b2c3d4e5f6a7b2c3d4e5f6a7b2c3d4e5f6a7b2c3',
    commitMessage: 'feat(scaffold): add project scaffold command with template selection',
    files: [
      {
        file_path: 'src/commands/scaffold.ts',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 22,
        deletions: 0,
        lines: [
          { line_type: 'header', content: '@@ -0,0 +1,22 @@' },
          { line_type: 'added', content: "import { Command } from 'commander'", new_line_number: 1 },
          { line_type: 'added', content: "import { scaffoldProject } from '@/lib/scaffold'", new_line_number: 2 },
          { line_type: 'added', content: '', new_line_number: 3 },
          { line_type: 'added', content: "export type ScaffoldTemplate = 'react' | 'next' | 'go-api' | 'go-cli'", new_line_number: 4 },
          { line_type: 'added', content: '', new_line_number: 5 },
          { line_type: 'added', content: 'export const scaffoldCommand = new Command("scaffold")', new_line_number: 6 },
          { line_type: 'added', content: '  .description("Scaffold a new project from a template")', new_line_number: 7 },
          { line_type: 'added', content: '  .argument("<name>", "Project name")', new_line_number: 8 },
          { line_type: 'added', content: '  .option("-t, --template <template>", "Template to use", "react")', new_line_number: 9 },
          { line_type: 'added', content: '  .option("-d, --dir <dir>", "Output directory", ".")', new_line_number: 10 },
          { line_type: 'added', content: '  .action(async (name: string, opts: { template: ScaffoldTemplate; dir: string }) => {', new_line_number: 11 },
          { line_type: 'added', content: '    await scaffoldProject({ name, template: opts.template, outputDir: opts.dir })', new_line_number: 12 },
          { line_type: 'added', content: '  })', new_line_number: 13 },
        ],
      },
      {
        file_path: 'src/commands/index.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 3,
        deletions: 0,
        lines: [
          { line_type: 'header', content: '@@ -4,6 +4,9 @@' },
          { line_type: 'context', content: "import { buildCommand } from './build'", old_line_number: 4, new_line_number: 4 },
          { line_type: 'context', content: "import { testCommand } from './test'", old_line_number: 5, new_line_number: 5 },
          { line_type: 'added', content: "import { scaffoldCommand } from './scaffold'", new_line_number: 6 },
          { line_type: 'context', content: '', old_line_number: 6, new_line_number: 7 },
          { line_type: 'context', content: 'program', old_line_number: 7, new_line_number: 8 },
          { line_type: 'added', content: '  .addCommand(scaffoldCommand)', new_line_number: 9 },
          { line_type: 'context', content: '  .addCommand(buildCommand)', old_line_number: 8, new_line_number: 10 },
          { line_type: 'context', content: '  .addCommand(testCommand)', old_line_number: 9, new_line_number: 11 },
          { line_type: 'added', content: '  .parse(process.argv)', new_line_number: 12 },
        ],
      },
    ],
    totalFiles: 2,
    totalAdditions: 25,
    totalDeletions: 0,
  },

  // ws-fix — fix/toolbar-crash — tiny bugfix
  'ws-fix': {
    commitHash: 'c3d4e5f6a7b8c3d4e5f6a7b8c3d4e5f6a7b8c3d4',
    commitMessage: 'fix(toolbar): guard against null ref before calling focus()',
    files: [
      {
        file_path: 'src/components/ui/toolbar.tsx',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 3,
        deletions: 1,
        lines: [
          { line_type: 'header', content: '@@ -42,7 +42,9 @@' },
          { line_type: 'context', content: '  function handleKeyDown(e: React.KeyboardEvent) {', old_line_number: 42, new_line_number: 42 },
          { line_type: 'context', content: "    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return", old_line_number: 43, new_line_number: 43 },
          { line_type: 'removed', content: '    itemRefs.current[nextIdx]?.focus()', old_line_number: 44 },
          { line_type: 'added', content: '    const nextEl = itemRefs.current[nextIdx]', new_line_number: 44 },
          { line_type: 'added', content: '    if (nextEl == null) return', new_line_number: 45 },
          { line_type: 'added', content: '    nextEl.focus()', new_line_number: 46 },
          { line_type: 'context', content: '  }', old_line_number: 45, new_line_number: 47 },
        ],
      },
    ],
    totalFiles: 1,
    totalAdditions: 3,
    totalDeletions: 1,
  },

  // ws2 — feature/api-backend — auth system, medium
  ws2: {
    commitHash: 'd4e5f6a7b8c9d4e5f6a7b8c9d4e5f6a7b8c9d4e5',
    commitMessage: 'feat(auth): add JWT-based session management with refresh token rotation',
    files: [
      {
        file_path: 'src/lib/jwt.ts',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 38,
        deletions: 0,
        lines: [
          { line_type: 'header', content: '@@ -0,0 +1,38 @@' },
          { line_type: 'added', content: "import { SignJWT, jwtVerify, type JWTPayload } from 'jose'", new_line_number: 1 },
          { line_type: 'added', content: '', new_line_number: 2 },
          { line_type: 'added', content: 'const ACCESS_SECRET = new TextEncoder().encode(process.env.JWT_ACCESS_SECRET)', new_line_number: 3 },
          { line_type: 'added', content: 'const REFRESH_SECRET = new TextEncoder().encode(process.env.JWT_REFRESH_SECRET)', new_line_number: 4 },
          { line_type: 'added', content: '', new_line_number: 5 },
          { line_type: 'added', content: 'export interface TokenPayload extends JWTPayload {', new_line_number: 6 },
          { line_type: 'added', content: '  userId: string', new_line_number: 7 },
          { line_type: 'added', content: '  role: "user" | "admin"', new_line_number: 8 },
          { line_type: 'added', content: '  sessionId: string', new_line_number: 9 },
          { line_type: 'added', content: '}', new_line_number: 10 },
          { line_type: 'added', content: '', new_line_number: 11 },
          { line_type: 'added', content: 'export async function signAccessToken(payload: Omit<TokenPayload, keyof JWTPayload>) {', new_line_number: 12 },
          { line_type: 'added', content: '  return new SignJWT(payload)', new_line_number: 13 },
          { line_type: 'added', content: '    .setProtectedHeader({ alg: "HS256" })', new_line_number: 14 },
          { line_type: 'added', content: "    .setExpirationTime('15m')", new_line_number: 15 },
          { line_type: 'added', content: '    .setIssuedAt()', new_line_number: 16 },
          { line_type: 'added', content: '    .sign(ACCESS_SECRET)', new_line_number: 17 },
          { line_type: 'added', content: '}', new_line_number: 18 },
          { line_type: 'added', content: '', new_line_number: 19 },
          { line_type: 'added', content: 'export async function signRefreshToken(payload: Omit<TokenPayload, keyof JWTPayload>) {', new_line_number: 20 },
          { line_type: 'added', content: '  return new SignJWT(payload)', new_line_number: 21 },
          { line_type: 'added', content: '    .setProtectedHeader({ alg: "HS256" })', new_line_number: 22 },
          { line_type: 'added', content: "    .setExpirationTime('7d')", new_line_number: 23 },
          { line_type: 'added', content: '    .setIssuedAt()', new_line_number: 24 },
          { line_type: 'added', content: '    .sign(REFRESH_SECRET)', new_line_number: 25 },
          { line_type: 'added', content: '}', new_line_number: 26 },
          { line_type: 'added', content: '', new_line_number: 27 },
          { line_type: 'added', content: 'export async function verifyAccessToken(token: string): Promise<TokenPayload> {', new_line_number: 28 },
          { line_type: 'added', content: '  const { payload } = await jwtVerify(token, ACCESS_SECRET)', new_line_number: 29 },
          { line_type: 'added', content: '  return payload as TokenPayload', new_line_number: 30 },
          { line_type: 'added', content: '}', new_line_number: 31 },
        ],
      },
      {
        file_path: 'src/api/auth/login.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 26,
        deletions: 12,
        lines: [
          { line_type: 'header', content: '@@ -1,22 +1,36 @@' },
          { line_type: 'removed', content: "import { compareSync } from 'bcrypt'", old_line_number: 1 },
          { line_type: 'removed', content: "import { sign } from 'jsonwebtoken'", old_line_number: 2 },
          { line_type: 'added', content: "import { compareSync } from 'bcryptjs'", new_line_number: 1 },
          { line_type: 'added', content: "import { signAccessToken, signRefreshToken } from '@/lib/jwt'", new_line_number: 2 },
          { line_type: 'context', content: "import { db } from '@/lib/db'", old_line_number: 3, new_line_number: 3 },
          { line_type: 'context', content: '', old_line_number: 4, new_line_number: 4 },
          { line_type: 'context', content: 'export async function loginHandler(req: Request, res: Response) {', old_line_number: 5, new_line_number: 5 },
          { line_type: 'context', content: '  const { email, password } = req.body', old_line_number: 6, new_line_number: 6 },
          { line_type: 'context', content: '  const user = await db.user.findUnique({ where: { email } })', old_line_number: 7, new_line_number: 7 },
          { line_type: 'context', content: "  if (!user) return res.status(401).json({ error: 'Invalid credentials' })", old_line_number: 8, new_line_number: 8 },
          { line_type: 'context', content: '  const valid = compareSync(password, user.passwordHash)', old_line_number: 9, new_line_number: 9 },
          { line_type: 'context', content: "  if (!valid) return res.status(401).json({ error: 'Invalid credentials' })", old_line_number: 10, new_line_number: 10 },
          { line_type: 'removed', content: '  const token = sign({ userId: user.id }, process.env.JWT_SECRET!, { expiresIn: "1d" })', old_line_number: 11 },
          { line_type: 'removed', content: '  res.json({ token })', old_line_number: 12 },
          { line_type: 'added', content: '  const sessionId = crypto.randomUUID()', new_line_number: 11 },
          { line_type: 'added', content: '  const payload = { userId: user.id, role: user.role, sessionId }', new_line_number: 12 },
          { line_type: 'added', content: '  const [accessToken, refreshToken] = await Promise.all([', new_line_number: 13 },
          { line_type: 'added', content: '    signAccessToken(payload),', new_line_number: 14 },
          { line_type: 'added', content: '    signRefreshToken(payload),', new_line_number: 15 },
          { line_type: 'added', content: '  ])', new_line_number: 16 },
          { line_type: 'added', content: '  await db.session.create({ data: { id: sessionId, userId: user.id } })', new_line_number: 17 },
          { line_type: 'added', content: "  res.cookie('refresh_token', refreshToken, { httpOnly: true, sameSite: 'strict' })", new_line_number: 18 },
          { line_type: 'added', content: '  res.json({ accessToken })', new_line_number: 19 },
          { line_type: 'context', content: '}', old_line_number: 13, new_line_number: 20 },
        ],
      },
      {
        file_path: 'src/middleware/auth.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 14,
        deletions: 8,
        lines: [
          { line_type: 'header', content: '@@ -1,16 +1,22 @@' },
          { line_type: 'removed', content: "import { verify } from 'jsonwebtoken'", old_line_number: 1 },
          { line_type: 'added', content: "import { verifyAccessToken } from '@/lib/jwt'", new_line_number: 1 },
          { line_type: 'context', content: '', old_line_number: 2, new_line_number: 2 },
          { line_type: 'context', content: 'export async function authMiddleware(req: Request, res: Response, next: NextFunction) {', old_line_number: 3, new_line_number: 3 },
          { line_type: 'removed', content: "  const token = req.headers.authorization?.split(' ')[1]", old_line_number: 4 },
          { line_type: 'removed', content: "  if (!token) return res.status(401).json({ error: 'No token' })", old_line_number: 5 },
          { line_type: 'removed', content: '  try {', old_line_number: 6 },
          { line_type: 'removed', content: '    req.user = verify(token, process.env.JWT_SECRET!) as TokenPayload', old_line_number: 7 },
          { line_type: 'removed', content: '    next()', old_line_number: 8 },
          { line_type: 'removed', content: '  } catch {', old_line_number: 9 },
          { line_type: 'removed', content: "    res.status(401).json({ error: 'Invalid token' })", old_line_number: 10 },
          { line_type: 'removed', content: '  }', old_line_number: 11 },
          { line_type: 'added', content: "  const authHeader = req.headers.authorization", new_line_number: 4 },
          { line_type: 'added', content: "  const token = authHeader?.startsWith('Bearer ') ? authHeader.slice(7) : null", new_line_number: 5 },
          { line_type: 'added', content: "  if (!token) return res.status(401).json({ error: 'No token' })", new_line_number: 6 },
          { line_type: 'added', content: '  try {', new_line_number: 7 },
          { line_type: 'added', content: '    req.user = await verifyAccessToken(token)', new_line_number: 8 },
          { line_type: 'added', content: '    next()', new_line_number: 9 },
          { line_type: 'added', content: '  } catch {', new_line_number: 10 },
          { line_type: 'added', content: "    res.status(401).json({ error: 'Invalid or expired token' })", new_line_number: 11 },
          { line_type: 'added', content: '  }', new_line_number: 12 },
          { line_type: 'context', content: '}', old_line_number: 12, new_line_number: 13 },
        ],
      },
    ],
    totalFiles: 3,
    totalAdditions: 78,
    totalDeletions: 20,
  },

  // ws4 — feature/ws-channels — WebSocket feature
  ws4: {
    commitHash: 'e5f6a7b8c9d0e5f6a7b8c9d0e5f6a7b8c9d0e5f6',
    commitMessage: 'feat(ws): add typed channel system with subscription management',
    files: [
      {
        file_path: 'src/lib/ws/channel.ts',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 52,
        deletions: 0,
        lines: [
          { line_type: 'header', content: '@@ -0,0 +1,52 @@' },
          { line_type: 'added', content: 'export type ChannelEvent<T = unknown> = {', new_line_number: 1 },
          { line_type: 'added', content: '  type: string', new_line_number: 2 },
          { line_type: 'added', content: '  payload: T', new_line_number: 3 },
          { line_type: 'added', content: '  channelId: string', new_line_number: 4 },
          { line_type: 'added', content: '  timestamp: number', new_line_number: 5 },
          { line_type: 'added', content: '}', new_line_number: 6 },
          { line_type: 'added', content: '', new_line_number: 7 },
          { line_type: 'added', content: 'export type Subscriber<T> = (event: ChannelEvent<T>) => void', new_line_number: 8 },
          { line_type: 'added', content: '', new_line_number: 9 },
          { line_type: 'added', content: 'export class Channel<T = unknown> {', new_line_number: 10 },
          { line_type: 'added', content: '  private subscribers = new Set<Subscriber<T>>()', new_line_number: 11 },
          { line_type: 'added', content: '  readonly id: string', new_line_number: 12 },
          { line_type: 'added', content: '', new_line_number: 13 },
          { line_type: 'added', content: '  constructor(id: string) {', new_line_number: 14 },
          { line_type: 'added', content: '    this.id = id', new_line_number: 15 },
          { line_type: 'added', content: '  }', new_line_number: 16 },
          { line_type: 'added', content: '', new_line_number: 17 },
          { line_type: 'added', content: '  subscribe(fn: Subscriber<T>): () => void {', new_line_number: 18 },
          { line_type: 'added', content: '    this.subscribers.add(fn)', new_line_number: 19 },
          { line_type: 'added', content: '    return () => this.subscribers.delete(fn)', new_line_number: 20 },
          { line_type: 'added', content: '  }', new_line_number: 21 },
          { line_type: 'added', content: '', new_line_number: 22 },
          { line_type: 'added', content: '  emit(type: string, payload: T): void {', new_line_number: 23 },
          { line_type: 'added', content: '    const event: ChannelEvent<T> = { type, payload, channelId: this.id, timestamp: Date.now() }', new_line_number: 24 },
          { line_type: 'added', content: '    this.subscribers.forEach(fn => fn(event))', new_line_number: 25 },
          { line_type: 'added', content: '  }', new_line_number: 26 },
          { line_type: 'added', content: '', new_line_number: 27 },
          { line_type: 'added', content: '  clear(): void { this.subscribers.clear() }', new_line_number: 28 },
          { line_type: 'added', content: '}', new_line_number: 29 },
        ],
      },
      {
        file_path: 'src/lib/ws/ws-client.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 30,
        deletions: 18,
        lines: [
          { line_type: 'header', content: '@@ -1,28 +1,40 @@' },
          { line_type: 'removed', content: "import { EventEmitter } from 'events'", old_line_number: 1 },
          { line_type: 'added', content: "import { Channel, type ChannelEvent } from './channel'", new_line_number: 1 },
          { line_type: 'context', content: '', old_line_number: 2, new_line_number: 2 },
          { line_type: 'removed', content: 'export class WsClient extends EventEmitter {', old_line_number: 3 },
          { line_type: 'removed', content: '  private ws: WebSocket | null = null', old_line_number: 4 },
          { line_type: 'removed', content: '  private reconnectAttempts = 0', old_line_number: 5 },
          { line_type: 'added', content: 'export class WsClient {', new_line_number: 3 },
          { line_type: 'added', content: '  private ws: WebSocket | null = null', new_line_number: 4 },
          { line_type: 'added', content: '  private reconnectAttempts = 0', new_line_number: 5 },
          { line_type: 'added', content: '  private channels = new Map<string, Channel>()', new_line_number: 6 },
          { line_type: 'context', content: '', old_line_number: 6, new_line_number: 7 },
          { line_type: 'removed', content: '  on(event: string, listener: (data: unknown) => void) {', old_line_number: 7 },
          { line_type: 'removed', content: '    return super.on(event, listener)', old_line_number: 8 },
          { line_type: 'removed', content: '  }', old_line_number: 9 },
          { line_type: 'added', content: '  channel<T>(id: string): Channel<T> {', new_line_number: 8 },
          { line_type: 'added', content: '    if (!this.channels.has(id)) {', new_line_number: 9 },
          { line_type: 'added', content: '      this.channels.set(id, new Channel(id))', new_line_number: 10 },
          { line_type: 'added', content: '    }', new_line_number: 11 },
          { line_type: 'added', content: '    return this.channels.get(id) as Channel<T>', new_line_number: 12 },
          { line_type: 'added', content: '  }', new_line_number: 13 },
          { line_type: 'context', content: '', old_line_number: 10, new_line_number: 14 },
          { line_type: 'context', content: '  connect(url: string): void {', old_line_number: 11, new_line_number: 15 },
          { line_type: 'context', content: '    this.ws = new WebSocket(url)', old_line_number: 12, new_line_number: 16 },
          { line_type: 'removed', content: "    this.ws.onmessage = (e) => this.emit('message', JSON.parse(e.data))", old_line_number: 13 },
          { line_type: 'added', content: '    this.ws.onmessage = (e) => {', new_line_number: 17 },
          { line_type: 'added', content: '      const event = JSON.parse(e.data) as ChannelEvent', new_line_number: 18 },
          { line_type: 'added', content: '      this.channels.get(event.channelId)?.emit(event.type, event.payload)', new_line_number: 19 },
          { line_type: 'added', content: '    }', new_line_number: 20 },
          { line_type: 'context', content: '  }', old_line_number: 14, new_line_number: 21 },
        ],
      },
      {
        file_path: 'src/hooks/use-ws-channel.ts',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 28,
        deletions: 0,
        lines: [
          { line_type: 'header', content: '@@ -0,0 +1,28 @@' },
          { line_type: 'added', content: "import { useEffect, useRef, useState } from 'react'", new_line_number: 1 },
          { line_type: 'added', content: "import type { ChannelEvent } from '@/lib/ws/channel'", new_line_number: 2 },
          { line_type: 'added', content: "import { wsClient } from '@/lib/ws/ws-client'", new_line_number: 3 },
          { line_type: 'added', content: '', new_line_number: 4 },
          { line_type: 'added', content: 'export function useWsChannel<T>(channelId: string) {', new_line_number: 5 },
          { line_type: 'added', content: '  const [lastEvent, setLastEvent] = useState<ChannelEvent<T> | null>(null)', new_line_number: 6 },
          { line_type: 'added', content: '  const channelRef = useRef(wsClient.channel<T>(channelId))', new_line_number: 7 },
          { line_type: 'added', content: '', new_line_number: 8 },
          { line_type: 'added', content: '  useEffect(() => {', new_line_number: 9 },
          { line_type: 'added', content: '    const unsubscribe = channelRef.current.subscribe(event => {', new_line_number: 10 },
          { line_type: 'added', content: '      setLastEvent(event)', new_line_number: 11 },
          { line_type: 'added', content: '    })', new_line_number: 12 },
          { line_type: 'added', content: '    return unsubscribe', new_line_number: 13 },
          { line_type: 'added', content: '  }, [channelId])', new_line_number: 14 },
          { line_type: 'added', content: '', new_line_number: 15 },
          { line_type: 'added', content: '  return lastEvent', new_line_number: 16 },
          { line_type: 'added', content: '}', new_line_number: 17 },
        ],
      },
    ],
    totalFiles: 3,
    totalAdditions: 110,
    totalDeletions: 18,
  },
}

// ws5 extreme scenario — generated programmatically
function buildWs5Diff(): MultiFileDiff {
  const bigFileLines = generateLargeFileDiff('src/lib/query/query-layer.ts', 260)
  const mediumFileLines = generateLargeFileDiff('src/lib/query/cache-manager.ts', 120)

  return {
    commitHash: 'f6a7b8c9d0e1f6a7b8c9d0e1f6a7b8c9d0e1f6a7',
    commitMessage: 'refactor(query-layer): replace legacy fetch + global cache with typed QueryLayer + CacheManager',
    files: [
      {
        file_path: 'src/lib/query/query-layer.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 65234,
        deletions: 44821,
        lines: bigFileLines,
      },
      {
        file_path: 'src/lib/query/cache-manager.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 18248,
        deletions: 20101,
        lines: mediumFileLines,
      },
      {
        file_path: 'src/lib/query/index.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 12,
        deletions: 28,
        lines: [
          { line_type: 'header', content: '@@ -1,30 +1,14 @@' },
          { line_type: 'removed', content: "export { legacyFetch } from './legacy-fetch'", old_line_number: 1 },
          { line_type: 'removed', content: "export { globalCache, clearCache, getCachedOrFetch } from './global-cache'", old_line_number: 2 },
          { line_type: 'removed', content: "export { createRequest } from './request-factory'", old_line_number: 3 },
          { line_type: 'removed', content: "export type { RequestConfig } from './request-factory'", old_line_number: 4 },
          { line_type: 'removed', content: "export { retryWithBackoff } from './retry'", old_line_number: 5 },
          { line_type: 'removed', content: "export { batchRequests } from './batch'", old_line_number: 6 },
          { line_type: 'added', content: "export { QueryLayer, createQueryHook } from './query-layer'", new_line_number: 1 },
          { line_type: 'added', content: "export { CacheManager } from './cache-manager'", new_line_number: 2 },
          { line_type: 'added', content: "export type { QueryLayerConfig } from './query-layer'", new_line_number: 3 },
          { line_type: 'added', content: "export type { CacheEntry, CacheOptions } from './cache-manager'", new_line_number: 4 },
        ],
      },
      {
        file_path: 'src/lib/query/legacy-fetch.ts',
        is_new: false,
        is_deleted: true,
        is_renamed: false,
        additions: 0,
        deletions: 5089,
        lines: [
          { line_type: 'header', content: '@@ -1,5089 +0,0 @@' },
          { line_type: 'removed', content: '// This file has been replaced by QueryLayer. Delete me.', old_line_number: 1 },
          { line_type: 'removed', content: "import { globalCache } from './global-cache'", old_line_number: 2 },
          { line_type: 'removed', content: '', old_line_number: 3 },
          { line_type: 'removed', content: '// ... (5086 more lines removed)', old_line_number: 4 },
        ],
      },
    ],
    totalFiles: 4,
    totalAdditions: 103482,
    totalDeletions: 88910,
  }
}

const DIFF_FALLBACK: MultiFileDiff = {
  commitHash: '0000000000000000000000000000000000000000',
  commitMessage: 'Generic changes',
  files: [
    {
      file_path: 'src/index.ts',
      is_new: false,
      is_deleted: false,
      is_renamed: false,
      additions: 4,
      deletions: 2,
      lines: [
        { line_type: 'header', content: '@@ -1,4 +1,6 @@' },
        { line_type: 'context', content: "import { App } from './app'", old_line_number: 1, new_line_number: 1 },
        { line_type: 'removed', content: 'const port = 3000', old_line_number: 2 },
        { line_type: 'added', content: "const port = Number(process.env.PORT ?? '3000')", new_line_number: 2 },
        { line_type: 'added', content: "const host = process.env.HOST ?? '0.0.0.0'", new_line_number: 3 },
        { line_type: 'context', content: 'new App().listen(port)', old_line_number: 3, new_line_number: 4 },
      ],
    },
    {
      file_path: 'src/app.ts',
      is_new: false,
      is_deleted: false,
      is_renamed: false,
      additions: 3,
      deletions: 1,
      lines: [
        { line_type: 'header', content: '@@ -8,5 +8,7 @@' },
        { line_type: 'context', content: '  listen(port: number) {', old_line_number: 8, new_line_number: 8 },
        { line_type: 'removed', content: "    console.log(`Listening on port ${port}`)", old_line_number: 9 },
        { line_type: 'added', content: "    console.info(`Server started on ${host}:${port}`)", new_line_number: 9 },
        { line_type: 'added', content: "    console.info(`Environment: ${process.env.NODE_ENV ?? 'development'}`)", new_line_number: 10 },
        { line_type: 'context', content: '  }', old_line_number: 10, new_line_number: 11 },
      ],
    },
  ],
  totalFiles: 2,
  totalAdditions: 7,
  totalDeletions: 3,
}

export function getMockBranchDiff(wsId: string): MultiFileDiff {
  if (wsId === 'ws5') return buildWs5Diff()
  return DIFFS[wsId] ?? DIFF_FALLBACK
}

// ---------------------------------------------------------------------------
// Review threads
// ---------------------------------------------------------------------------

const THREADS: Record<string, ReviewThread[]> = {
  ws3: [
    thread('t-ws3-1', 'src/features/settings/lib/design-tokens.ts', 9, 'right', [
      msg('m-ws3-1-1', null, false, 'Should we use a `Map` instead of an array here for O(1) lookup by name?', '2026-05-20T09:14:00Z'),
      msg('m-ws3-1-2', 'Aria', true, "Array is fine for a small, static list of tokens. If you later need lookup by name, a `Record<string, DesignToken>` would be more ergonomic than a `Map`. For now I'd keep it simple.", '2026-05-20T09:15:12Z'),
      msg('m-ws3-1-3', null, false, 'Makes sense, leaving as-is then.', '2026-05-20T09:18:44Z'),
    ], true),
    thread('t-ws3-2', 'src/features/settings/components/theme-provider.tsx', 22, 'right', [
      msg('m-ws3-2-1', 'Morgan', false, 'The `useEffect` injects a `<style>` tag on every colorScheme change — should we check if the content has actually changed before updating `textContent`?', '2026-05-20T10:03:00Z'),
      msg('m-ws3-2-2', null, false, 'Good catch. Adding an equality guard now.', '2026-05-20T10:08:00Z'),
    ], false),
    thread('t-ws3-3', 'src/features/settings/components/settings-panel.tsx', 21, 'right', [
      msg('m-ws3-3-1', 'Aria', true, 'The `ColorSchemeSelector` component doesn\'t exist yet — is this wiring up a stub or is it being added elsewhere in this PR?', '2026-05-20T10:30:00Z'),
      msg('m-ws3-3-2', null, false, "It's in a follow-up PR. I added a `// TODO` comment in the implementation file.", '2026-05-20T10:35:00Z'),
      msg('m-ws3-3-3', 'Aria', true, 'Got it. Worth adding a stub export so TypeScript doesn\'t complain in CI.', '2026-05-20T10:36:30Z'),
    ], false),
    thread('t-ws3-4', 'src/features/settings/lib/settings-store.ts', 25, 'right', [
      msg('m-ws3-4-1', null, false, 'Migrating from `create` to `create + immer` — is there a migration guide for existing persisted state?', '2026-05-20T11:00:00Z'),
      msg('m-ws3-4-2', 'Morgan', false, 'I checked — the shape is the same so deserialization will just work. No migration needed.', '2026-05-20T11:05:00Z'),
    ], true),
    thread('t-ws3-5', 'src/features/settings/components/theme-provider.tsx', 27, 'right', [
      msg('m-ws3-5-1', null, false, "Nit: `:root { ... }` already has a trailing `\\n` from `cssVariableString` — we'll get a double newline in the generated `<style>` tag.", '2026-05-20T11:20:00Z'),
    ], false),
  ],

  ws1: [
    thread('t-ws1-1', 'src/commands/scaffold.ts', 11, 'right', [
      msg('m-ws1-1-1', 'Aria', true, 'The action callback type annotation for `opts` is redundant since Commander infers it. You can drop it to reduce noise, though being explicit is also fine.', '2026-05-28T14:10:00Z'),
    ], false),
  ],

  ws2: [
    thread('t-ws2-1', 'src/lib/jwt.ts', 3, 'right', [
      msg('m-ws2-1-1', null, false, 'Should these secrets be validated at startup rather than lazily? If `JWT_ACCESS_SECRET` is undefined the `TextEncoder` will encode `"undefined"` as the secret.', '2026-05-22T08:45:00Z'),
      msg('m-ws2-1-2', 'Aria', true, 'Critical catch. You should add a startup guard: `if (!process.env.JWT_ACCESS_SECRET) throw new Error("JWT_ACCESS_SECRET is required")`. Otherwise the token signing will silently use a predictable key.', '2026-05-22T08:46:30Z'),
      msg('m-ws2-1-3', null, false, 'Adding the guard now.', '2026-05-22T08:50:00Z'),
    ], false),
    thread('t-ws2-2', 'src/api/auth/login.ts', 17, 'right', [
      msg('m-ws2-2-1', 'Morgan', false, 'Should the session also store the user\'s IP and user-agent for anomaly detection?', '2026-05-22T09:10:00Z'),
      msg('m-ws2-2-2', null, false, "Out of scope for this PR — opening a follow-up issue.", '2026-05-22T09:15:00Z'),
    ], true),
    thread('t-ws2-3', 'src/middleware/auth.ts', 11, 'right', [
      msg('m-ws2-3-1', 'Aria', true, "The error message change from `'Invalid token'` to `'Invalid or expired token'` leaks information about *why* the token was rejected. Attackers can distinguish expired from tampered tokens. Consider a generic `'Unauthorized'`.`", '2026-05-22T09:30:00Z'),
      msg('m-ws2-3-2', null, false, 'Good point. Changing to generic message.', '2026-05-22T09:35:00Z'),
      msg('m-ws2-3-3', 'Aria', true, 'Confirmed, that resolves the concern.', '2026-05-22T09:36:00Z'),
    ], false),
    thread('t-ws2-4', 'src/lib/jwt.ts', 23, 'right', [
      msg('m-ws2-4-1', null, false, "7-day refresh token feels long. Could we make this configurable via env var?", '2026-05-22T10:00:00Z'),
      msg('m-ws2-4-2', 'Morgan', false, 'Agreed — `JWT_REFRESH_TOKEN_TTL` env var with a 7d default would be clean.', '2026-05-22T10:05:00Z'),
    ], false),
  ],

  ws4: [
    thread('t-ws4-1', 'src/lib/ws/channel.ts', 11, 'right', [
      msg('m-ws4-1-1', 'Aria', true, '`Set<Subscriber<T>>` is correct here — prevents duplicate subscriptions. One thing to watch: if users subscribe with inline arrow functions, each call creates a new reference so they\'ll accumulate. Consider documenting this caveat or providing a `subscribeOnce` helper.', '2026-05-25T13:00:00Z'),
      msg('m-ws4-1-2', null, false, 'Good note. Adding a JSDoc comment about stable references.', '2026-05-25T13:04:00Z'),
    ], false),
    thread('t-ws4-2', 'src/lib/ws/ws-client.ts', 18, 'right', [
      msg('m-ws4-2-1', null, false, 'What happens if `event.channelId` refers to a channel that hasn\'t been subscribed to yet? The message will be silently dropped.', '2026-05-25T13:20:00Z'),
      msg('m-ws4-2-2', 'Morgan', false, 'That\'s intentional — messages for channels with no subscribers are fire-and-forget. Maybe log a debug warning though.', '2026-05-25T13:25:00Z'),
      msg('m-ws4-2-3', null, false, 'Added debug log behind a `__DEV__` flag.', '2026-05-25T13:30:00Z'),
    ], true),
    thread('t-ws4-3', 'src/hooks/use-ws-channel.ts', 7, 'right', [
      msg('m-ws4-3-1', 'Aria', true, '`wsClient.channel<T>(channelId)` is called during render (inside `useRef` initializer). This is fine on the first render but if `channelId` changes the ref won\'t update. The `channelId` in the dep array handles re-subscribing, but `channelRef.current` will still point to the old channel. Recommend `const channel = wsClient.channel<T>(channelId)` inside the effect instead.', '2026-05-25T14:00:00Z'),
      msg('m-ws4-3-2', null, false, 'Great catch. Refactoring to put the channel lookup inside the effect.', '2026-05-25T14:08:00Z'),
    ], false),
    thread('t-ws4-4', 'src/lib/ws/channel.ts', 23, 'right', [
      msg('m-ws4-4-1', null, false, 'Should `emit` be synchronous or should we queue events to avoid re-entrant subscriber calls?', '2026-05-25T14:30:00Z'),
      msg('m-ws4-4-2', 'Aria', true, 'Synchronous is simpler and predictable for most use cases. If you need queueing later, wrapping `forEach` with `queueMicrotask` is an easy upgrade. I\'d keep it sync for now.', '2026-05-25T14:32:00Z'),
    ], true),
  ],

  ws5: [
    thread('t-ws5-1', 'src/lib/query/query-layer.ts', 28, 'right', [
      msg('m-ws5-1-1', 'Aria', true, 'The spread `{ ...DEFAULT_CONFIG, ...config }` will silently ignore unknown keys passed by callers. Consider using a validation library (e.g. zod) or at least a runtime check to catch misconfigurations early.', '2026-05-15T08:00:00Z'),
      msg('m-ws5-1-2', null, false, "Adding a `validateConfig` helper that narrows the input type.", '2026-05-15T08:05:00Z'),
      msg('m-ws5-1-3', 'Morgan', false, 'zod might be overkill here. A simple type-narrowing function should be enough since the type system already catches most mistakes at compile time.', '2026-05-15T08:08:00Z'),
      msg('m-ws5-1-4', 'Aria', true, "Agreed with Morgan — a narrow helper is sufficient. The main value is catching `undefined` where a number is expected at runtime.", '2026-05-15T08:09:30Z'),
    ], true),
    thread('t-ws5-2', 'src/lib/query/query-layer.ts', 38, 'right', [
      msg('m-ws5-2-1', null, false, 'The timeout `Promise.race` doesn\'t clear the underlying timer if `fetcher` resolves first — this leaks a timer for every successful fast request.', '2026-05-15T09:00:00Z'),
      msg('m-ws5-2-2', 'Aria', true, 'Critical bug. The timeout `setTimeout` needs to be stored and cleared in the `finally` block. Otherwise under high load you\'ll accumulate thousands of pending timers. Here\'s the fix:\n```ts\nconst timeoutId = setTimeout(...)\ntry {\n  return await fetcher()\n} finally {\n  clearTimeout(timeoutId)\n}\n```', '2026-05-15T09:02:00Z'),
      msg('m-ws5-2-3', null, false, 'Fixed. Good catch.', '2026-05-15T09:10:00Z'),
    ], true),
    thread('t-ws5-3', 'src/lib/query/query-layer.ts', 55, 'right', [
      msg('m-ws5-3-1', 'Morgan', false, "Why does `invalidate` call `invalidateQueries` but `abort` doesn't also invalidate? After aborting, the cache still has stale data.", '2026-05-15T10:00:00Z'),
      msg('m-ws5-3-2', null, false, 'Intentional — abort just cancels in-flight; the caller decides whether to also invalidate. Added a note in the JSDoc.', '2026-05-15T10:08:00Z'),
    ], false),
    thread('t-ws5-4', 'src/lib/query/query-layer.ts', 63, 'right', [
      msg('m-ws5-4-1', 'Aria', true, '`destroy()` calls `abortAll()` then `client.clear()` — but there\'s no guard preventing the instance from being used after `destroy()`. If a caller holds a reference and calls `fetch` after `destroy`, it will create a new `AbortController` and try to use the cleared `QueryClient`. Consider adding an `isDestroyed` flag.', '2026-05-15T10:30:00Z'),
      msg('m-ws5-4-2', null, false, 'Adding `private isDestroyed = false` guard at the top of `fetch`.', '2026-05-15T10:35:00Z'),
      msg('m-ws5-4-3', 'Aria', true, "Also throw (not just return early) so callers don't silently get undefined.", '2026-05-15T10:36:00Z'),
    ], false),
    thread('t-ws5-5', 'src/lib/query/cache-manager.ts', 14, 'right', [
      msg('m-ws5-5-1', null, false, 'Is `CacheEntry` exported from the index? I see it referenced but not re-exported.', '2026-05-15T11:00:00Z'),
      msg('m-ws5-5-2', 'Morgan', false, "It's in `src/lib/query/index.ts` line 4 — it was added as part of this PR.", '2026-05-15T11:02:00Z'),
    ], true),
    thread('t-ws5-6', 'src/lib/query/query-layer.ts', 72, 'right', [
      msg('m-ws5-6-1', 'Aria', true, 'The `createQueryHook` return type is `() => { data: T | undefined, error: Error | null, status: ... }`. With React 18 concurrent mode, `setData` and `setStatus` in the `.then/.catch` chain could trigger two separate renders. Wrapping them in `React.startTransition` or merging into a single state object would reduce flicker.', '2026-05-15T11:30:00Z'),
      msg('m-ws5-6-2', null, false, 'Merging into a single state object. Good call.', '2026-05-15T11:40:00Z'),
      msg('m-ws5-6-3', 'Morgan', false, 'Or just use `useReducer` — it batches updates automatically in React 18.', '2026-05-15T11:42:00Z'),
      msg('m-ws5-6-4', 'Aria', true, 'Both approaches work. `useReducer` might be cleaner given there are three related state fields.', '2026-05-15T11:44:00Z'),
    ], false),
    thread('t-ws5-7', 'src/lib/query/index.ts', 1, 'right', [
      msg('m-ws5-7-1', null, false, '`legacy-fetch.ts` is deleted but are there any consumers still importing from it? Running a grep before merging would be good.', '2026-05-15T12:00:00Z'),
      msg('m-ws5-7-2', 'Aria', true, "I ran the grep — there are 3 remaining callers in `src/features/dashboard/`. They need to be migrated before this can merge. I'd block this PR on that.", '2026-05-15T12:02:30Z'),
      msg('m-ws5-7-3', null, false, 'Migrating those files in the next commit.', '2026-05-15T12:10:00Z'),
    ], false),
    thread('t-ws5-8', 'src/lib/query/cache-manager.ts', 35, 'right', [
      msg('m-ws5-8-1', 'Morgan', false, 'Does `CacheManager` handle cache stampede (multiple concurrent requests for the same key before the first one resolves)? The old global cache had this problem.', '2026-05-15T13:00:00Z'),
      msg('m-ws5-8-2', 'Aria', true, 'It does not — this is a known limitation called a "thundering herd". The fix is to store a pending Promise per key and return the same promise to all concurrent callers. This is the standard `in-flight deduplication` pattern. Worth adding before this ships to production.', '2026-05-15T13:03:00Z'),
      msg('m-ws5-8-3', null, false, "Adding in-flight dedup map now. This is a real concern given the 100k+ line refactor touches all data-fetching paths.", '2026-05-15T13:15:00Z'),
      msg('m-ws5-8-4', 'Morgan', false, 'I can help review that when it\'s up.', '2026-05-15T13:16:00Z'),
    ], false),
  ],
}

export function getMockBranchReviewThreads(wsId: string): ReviewThread[] {
  return THREADS[wsId] ?? []
}

// ---------------------------------------------------------------------------
// PR descriptions
// ---------------------------------------------------------------------------

const DESCRIPTIONS: Record<string, string> = {
  ws3: `## Design Token System Rework

Replaces the ad-hoc CSS variable approach with a structured \`DesignToken\` type and a \`colorTokens\` registry. The new \`ThemeProvider\` supports both light/dark theming and colour schemes (\`default\`, \`ocean\`, \`forest\`, \`rose\`).

### Changes
- **\`design-tokens.ts\`** — typed token registry with \`cssVariableString\` helper
- **\`theme-provider.tsx\`** — adds \`ColorScheme\` support; injects dynamic \`<style>\` tag for runtime token overrides
- **\`settings-panel.tsx\`** — wires up the new \`ColorSchemeSelector\` (stub, full component in follow-up PR)
- **\`settings-store.ts\`** — migrates to immer; adds \`colorScheme\` state

### Testing
- [ ] Toggle theme in settings and verify CSS variables update
- [ ] Switch colour scheme and verify \`data-color-scheme\` attribute updates on \`<html>\`
- [ ] Verify no flash of unstyled content on initial load`,

  ws1: `## Add Project Scaffold Command

Adds a \`crowbar scaffold <name>\` CLI command with template selection (\`react\`, \`next\`, \`go-api\`, \`go-cli\`).

### Usage
\`\`\`
crowbar scaffold my-app --template next --dir ./projects
\`\`\`

### Changes
- **\`scaffold.ts\`** — new \`scaffoldCommand\` built on Commander
- **\`index.ts\`** — registers the command in the CLI entry point`,

  'ws-fix': `## Fix Toolbar Crash on Keyboard Navigation

Guards against a null ref before calling \`.focus()\` when navigating toolbar items with arrow keys. The crash occurred when the last item was focused and the user pressed ArrowRight.`,

  ws2: `## JWT-Based Session Management with Refresh Token Rotation

Replaces the single long-lived access token with a short-lived access token (15m) + long-lived refresh token (7d) pattern. Refresh tokens are stored in httpOnly cookies to prevent XSS theft.

### Security notes
- Access tokens: 15-minute expiry, HS256
- Refresh tokens: 7-day expiry, httpOnly + SameSite=strict cookie
- Sessions tracked in DB for revocation support

### Changes
- **\`jwt.ts\`** — new signing/verification helpers using \`jose\` (replaces \`jsonwebtoken\`)
- **\`login.ts\`** — issues access + refresh token pair on successful login
- **\`middleware/auth.ts\`** — verifies access tokens asynchronously

### Testing
- [ ] Login flow returns access token in body and sets refresh cookie
- [ ] Protected routes reject requests without valid access token
- [ ] Expired access tokens return 401`,

  ws4: `## Typed WebSocket Channel System

Replaces the EventEmitter-based WsClient with a typed \`Channel<T>\` abstraction. Each logical channel (e.g. \`"presence"\`, \`"cursors"\`) has its own typed subscription set, avoiding the stringly-typed \`on('message')\` pattern.

### Changes
- **\`channel.ts\`** — \`Channel<T>\` class with typed subscribe/emit/clear
- **\`ws-client.ts\`** — \`channel<T>(id)\` factory replaces EventEmitter inheritance
- **\`use-ws-channel.ts\`** — React hook for consuming a channel in components

### Usage
\`\`\`ts
const presenceEvent = useWsChannel<PresencePayload>('presence')
\`\`\``,

  ws5: `## Refactor: Replace Legacy Fetch + Global Cache with QueryLayer + CacheManager

This is a **major refactor** touching ~190k lines across the data-fetching layer. The legacy \`legacyFetch\` + \`globalCache\` approach was:
- Not type-safe
- Missing request cancellation
- Prone to cache stampede
- Untestable (global mutable state)

The new \`QueryLayer\` + \`CacheManager\` architecture addresses all of these.

### Key improvements
- **Type-safe fetch** via generic \`QueryLayer.fetch<T>\`
- **Request cancellation** via per-key \`AbortController\` map
- **Configurable timeout** with proper timer cleanup
- **In-flight deduplication** to prevent thundering herd (🚧 in progress)
- **React integration** via \`createQueryHook\` factory

### Migration status
- [x] Core \`QueryLayer\` and \`CacheManager\` implemented
- [x] \`legacy-fetch.ts\` deleted
- [ ] Migrate 3 remaining callers in \`src/features/dashboard/\`
- [ ] Add in-flight deduplication to CacheManager
- [ ] Update integration tests

> ⚠️ **Do not merge** until the dashboard migration and in-flight dedup are complete.`,
}

export function getMockBranchReviewDescription(wsId: string): string {
  return DESCRIPTIONS[wsId] ?? ''
}

// ---------------------------------------------------------------------------
// Workspace-specific conversations
// ---------------------------------------------------------------------------

export type BranchReviewChat = ReviewConversation

const CHATS: Record<string, BranchReviewChat[]> = {
  ws3: [
    { id: 'ws3-c1', title: 'Token naming conventions', age: '3h', isActive: true },
    { id: 'ws3-c2', title: 'CSS variable vs Tailwind tokens', age: '1d', isActive: false },
    { id: 'ws3-c3', title: 'ThemeProvider SSR concerns', age: '2d', isActive: false },
  ],
  ws5: [
    { id: 'ws5-c1', title: 'QueryLayer API design', age: '1h', isActive: true },
    { id: 'ws5-c2', title: 'AbortController memory leak', age: '4h', isActive: true },
    { id: 'ws5-c3', title: 'Cache stampede mitigation', age: '1d', isActive: false },
    { id: 'ws5-c4', title: 'Migration plan for dashboard callers', age: '2d', isActive: false },
  ],
  ws1: [
    { id: 'ws1-c1', title: 'Scaffold command spec', age: '6h', isActive: true },
  ],
  ws2: [
    { id: 'ws2-c1', title: 'Auth flow edge cases', age: '2h', isActive: true },
    { id: 'ws2-c2', title: 'JWT refresh strategy', age: '3d', isActive: false },
  ],
  ws4: [
    { id: 'ws4-c1', title: 'WebSocket reconnection logic', age: '5h', isActive: true },
    { id: 'ws4-c2', title: 'Message ordering guarantees', age: '1d', isActive: false },
  ],
}

const DEFAULT_CHATS: BranchReviewChat[] = [
  { id: 'default-c1', title: 'Implementation discussion', age: '1d', isActive: false },
]

export function getMockBranchReviewChats(wsId: string): BranchReviewChat[] {
  return CHATS[wsId] ?? DEFAULT_CHATS
}
