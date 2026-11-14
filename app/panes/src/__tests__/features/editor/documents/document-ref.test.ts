import { describe, expect, it } from 'vitest'
import { Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import {
  documentName,
  documentRefOf,
  documentUri,
  refFromUri,
  sameDocument,
  siblingRef,
} from '@/features/editor/documents/document-ref'

const host = (path: string): DocumentRef => ({ authority: 'host', path })

describe('documentUri / refFromUri', () => {
  it.each([
    ['a workspace file', workspaceRef('ws-1', 'src/index.ts')],
    ['a workspace file needing encoding', workspaceRef('ws with space/slash', 'a b/ç d.ts')],
    ['a host file', host('/tmp/s5/notes.md')],
    ['a host file with spaces and unicode', host('/Users/me/Ünïcode dir/file #1.md')],
  ])('round-trips %s', (_, ref) => {
    expect(refFromUri(documentUri(ref))).toEqual(ref)
  })

  it('round-trips the uri Monaco hands back for a model', () => {
    for (const ref of [workspaceRef('ws-1', 'a b/c.ts'), host('/tmp/a b/ü.md')]) {
      expect(refFromUri(Uri.parse(documentUri(ref)).toString())).toEqual(ref)
    }
  })

  it('keeps the workspace uri the editor has always keyed models by', () => {
    expect(documentUri(workspaceRef('ws 1', 'src/a b.ts'))).toBe(
      'crowbar://editor/ws%201/src%2Fa%20b.ts',
    )
    expect(documentUri(host('/tmp/a.md'))).toBe('crowbar://host/%2Ftmp%2Fa.md')
  })

  // Monaco models are one global table: the same relative path in two
  // worktrees, or the same path on two authorities, must never share one.
  it('gives every document its own uri', () => {
    const uris = [
      documentUri(workspaceRef('workspace-a', 'src/inventory.ts')),
      documentUri(workspaceRef('workspace-b', 'src/inventory.ts')),
      documentUri(workspaceRef('workspace-a', 'src/other.ts')),
      documentUri(host('/src/inventory.ts')),
    ]
    expect(new Set(uris).size).toBe(uris.length)
  })

  it('answers null for a uri that names no document', () => {
    expect(refFromUri('file:///tmp/a.ts')).toBeNull()
    expect(refFromUri('crowbar://editor/no-path')).toBeNull()
    expect(refFromUri('crowbar://host/')).toBeNull()
  })
})

describe('documentRefOf', () => {
  it('names the authority the buffer was opened with', () => {
    expect(documentRefOf({ authority: 'workspace', workspaceId: 'w1', path: 'a.ts' })).toEqual(
      workspaceRef('w1', 'a.ts'),
    )
    expect(documentRefOf({ authority: 'host', workspaceId: '', path: '/a.ts' })).toEqual(
      host('/a.ts'),
    )
  })
})

describe('siblingRef', () => {
  it('resolves a relative path against a host document’s directory', () => {
    expect(siblingRef(host('/a/b/c.md'), '../img/x.png')).toEqual(host('/a/img/x.png'))
    expect(siblingRef(host('/a/b/c.md'), './x.png')).toEqual(host('/a/b/x.png'))
    expect(siblingRef(host('/a/b/c.md'), '/etc/x.png')).toEqual(host('/etc/x.png'))
  })

  it('keeps a workspace document’s sibling relative to its worktree', () => {
    expect(siblingRef(workspaceRef('w1', 'docs/a.md'), '../img/x.png')).toEqual(
      workspaceRef('w1', 'img/x.png'),
    )
    expect(siblingRef(workspaceRef('w1', 'docs/a.md'), '/img/x.png')).toEqual(
      workspaceRef('w1', 'img/x.png'),
    )
    expect(siblingRef(workspaceRef('w1', 'README.md'), 'other.md#top')).toEqual(
      workspaceRef('w1', 'other.md'),
    )
  })
})

describe('sameDocument / documentName', () => {
  it('matches on authority, workspace and path', () => {
    expect(sameDocument(workspaceRef('w1', 'a.ts'), workspaceRef('w1', 'a.ts'))).toBe(true)
    expect(sameDocument(workspaceRef('w1', 'a.ts'), workspaceRef('w2', 'a.ts'))).toBe(false)
    expect(sameDocument(host('/a.ts'), host('/a.ts'))).toBe(true)
    expect(sameDocument(host('a.ts'), workspaceRef('', 'a.ts'))).toBe(false)
  })

  it('names a document by its last path segment', () => {
    expect(documentName(host('/tmp/s5/notes.md'))).toBe('notes.md')
    expect(documentName(workspaceRef('w1', 'README.md'))).toBe('README.md')
  })
})
