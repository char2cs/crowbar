import { describe, expect, it } from 'vitest'
import { documentPaneTarget } from '@/entries/document-pane-target'

describe('documentPaneTarget', () => {
  it.each([
    [
      'a workspace file opens in its rich view',
      { path: 'README.md' },
      { doc: { authority: 'workspace', workspaceId: 'ws1', path: 'README.md' }, preview: null },
    ],
    [
      'a picked file belongs to the host',
      { path: '/home/a.md', absolute: true },
      { doc: { authority: 'host', path: '/home/a.md' }, preview: null },
    ],
    [
      'a preview names how it renders',
      { path: 'site/index.html', preview: 'html' },
      {
        doc: { authority: 'workspace', workspaceId: 'ws1', path: 'site/index.html' },
        preview: 'html',
      },
    ],
    [
      'an unknown preview is the rich view',
      { path: 'a.md', preview: 'pdf' },
      { doc: { authority: 'workspace', workspaceId: 'ws1', path: 'a.md' }, preview: null },
    ],
    ['no path names nothing', { untitled: true }, null],
    ['no state names nothing', null, null],
  ])('%s', (_name, saved, expected) => {
    expect(documentPaneTarget('ws1', saved)).toEqual(expected)
  })
})
