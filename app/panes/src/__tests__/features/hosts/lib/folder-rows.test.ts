import { describe, expect, it } from 'vitest'
import type { BrowseEntry } from '@/features/hosts/lib/browse'
import {
  folderNoteText,
  folderRows,
  type FolderNote,
  type FolderRow,
  type Listing,
} from '@/features/hosts/lib/folder-rows'

const dir = (path: string): BrowseEntry => ({ name: path.split('/').pop()!, path, isDir: true })
const file = (path: string): BrowseEntry => ({ ...dir(path), isDir: false })
const many = (n: number, make: (path: string) => BrowseEntry, under = '') =>
  Array.from({ length: n }, (_, i) => make(`${under}/${make === dir ? 'd' : 'f'}${i}`))

const NOTHING_OPEN = new Set<string>()
const describeRow = (row: FolderRow) =>
  row.kind === 'note' ? `${row.level} note:${row.note.type}` : `${row.level} ${row.entry.path}`

describe('folderRows', () => {
  it('lists folders before files, each at its level, and nothing under a closed folder', () => {
    const rows = folderRows('/', [file('/notes.txt'), dir('/srv'), dir('/home')], {}, NOTHING_OPEN)

    expect(rows.map(describeRow)).toEqual(['1 /srv', '1 /home', '1 /notes.txt'])
    expect(rows.map((row) => row.kind === 'entry' && row.expanded)).toEqual([
      false,
      false,
      undefined,
    ])
  })

  it('an open folder shows its listing nested under it, or a note while it has none', () => {
    const listings: Record<string, Listing> = {
      '/home': { status: 'loaded', entries: [dir('/home/mateo')] },
      '/srv': { status: 'loading' },
    }
    const open = new Set(['/home', '/srv', '/tmp'])

    const rows = folderRows('/', [dir('/home'), dir('/srv'), dir('/tmp')], listings, open)

    expect(rows.map(describeRow)).toEqual([
      '1 /home',
      '2 /home/mateo',
      '1 /srv',
      '2 note:loading',
      '1 /tmp',
      '2 note:waiting',
    ])
  })

  // Folders are what can be chosen, so every one is shown; only the inert files are cut.
  it('a huge directory shows every folder and the first 500 files, then says how many are hidden', () => {
    const rows = folderRows('/', [...many(600, file), ...many(700, dir)], {}, NOTHING_OPEN)

    const entries = rows.flatMap((row) => (row.kind === 'entry' ? [row.entry] : []))
    expect(entries.filter((e) => e.isDir)).toHaveLength(700)
    expect(entries.filter((e) => !e.isDir)).toHaveLength(500)
    const last = rows.at(-1)!
    expect(last).toMatchObject({
      kind: 'note',
      level: 1,
      note: { type: 'more-files', hidden: 100 },
    })
    expect(folderNoteText((last as { note: FolderNote }).note, 'homelab')).toBe(
      '100 more files not shown',
    )
  })

  it('exactly 500 files are all shown; one more is said as one file', () => {
    expect(
      folderRows('/', many(500, file), {}, NOTHING_OPEN).every((r) => r.kind === 'entry'),
    ).toBe(true)
    const rows = folderRows('/', many(501, file), {}, NOTHING_OPEN)
    const last = rows.at(-1) as { note: FolderNote }
    expect(rows).toHaveLength(501)
    expect(folderNoteText(last.note, 'homelab')).toBe('1 more file not shown')
  })

  it('each open folder cuts its own files, at its own level', () => {
    const listings: Record<string, Listing> = {
      '/big': { status: 'loaded', entries: many(502, file, '/big') },
    }

    const rows = folderRows('/', [dir('/big'), file('/a.txt')], listings, new Set(['/big']))

    expect(rows.at(-2)).toMatchObject({ level: 2, note: { type: 'more-files', hidden: 2 } })
    expect(describeRow(rows.at(-1)!)).toBe('1 /a.txt')
  })
})

describe('folderNoteText', () => {
  it.each([
    [{ type: 'waiting' }, 'Waiting for homelab to answer'],
    [{ type: 'loading' }, 'Loading…'],
    [{ type: 'empty' }, 'Empty folder'],
    [{ type: 'failed', error: new Error('permission denied') }, "Can't open: permission denied"],
    [{ type: 'more-files', hidden: 3 }, '3 more files not shown'],
  ] as const)('%j reads %j', (note, text) => {
    expect(folderNoteText(note, 'homelab')).toBe(text)
  })
})
