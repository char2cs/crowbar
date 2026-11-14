import { describe, expect, it } from 'vitest'
import { calculateDisplayNames } from '@/features/tabs/utils/path-shortener'
import type { EditorContent } from '@/features/panes/types/pane-content'

function tab(id: string, workspaceId: string, path: string): EditorContent {
  return {
    id,
    type: 'editor',
    authority: 'workspace',
    path,
    name: path.split('/').pop() ?? path,
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    workspaceId,
  }
}

const workspaceNames = new Map([
  ['ws-a', 'feat/a'],
  ['ws-b', 'feat/b'],
])

function labels(...tabs: EditorContent[]): string[] {
  const names = calculateDisplayNames(tabs, workspaceNames)
  return tabs.map((t) => names.get(t.id) ?? '')
}

describe('calculateDisplayNames', () => {
  it('shows a lone file by its name', () => {
    expect(labels(tab('1', 'ws-a', 'src/pricing.ts'))).toEqual(['pricing.ts'])
  })

  it('tells same-named files apart by the parent directory that differs', () => {
    expect(labels(tab('1', 'ws-a', 'src/pricing.ts'), tab('2', 'ws-a', 'lib/pricing.ts'))).toEqual([
      '../src/pricing.ts',
      '../lib/pricing.ts',
    ])
  })

  it('tells the same path in two workspaces apart by workspace', () => {
    expect(labels(tab('1', 'ws-a', 'src/pricing.ts'), tab('2', 'ws-b', 'src/pricing.ts'))).toEqual([
      'pricing.ts (feat/a)',
      'pricing.ts (feat/b)',
    ])
  })

  it('names only the workspace twins when a third file differs by directory', () => {
    expect(
      labels(
        tab('1', 'ws-a', 'src/pricing.ts'),
        tab('2', 'ws-b', 'src/pricing.ts'),
        tab('3', 'ws-a', 'lib/pricing.ts'),
      ),
    ).toEqual(['../src/pricing.ts (feat/a)', '../src/pricing.ts (feat/b)', '../lib/pricing.ts'])
  })
})
