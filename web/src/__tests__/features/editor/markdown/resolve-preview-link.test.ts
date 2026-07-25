import { describe, expect, it } from 'vitest'
import { resolvePreviewLinkPath } from '@/features/editor/markdown/resolve-preview-link'

// Every path the preview hands to the daemon must be WORKSPACE-RELATIVE: the fs
// engine's safepath.Resolve rejects an absolute path outright (400
// ErrPathEscapesWorkspace), so a leading slash makes the link dead even with a
// real `exists`/open behind it.
describe('resolvePreviewLinkPath', () => {
  it('resolves a sibling link against the current file directory', () => {
    expect(resolvePreviewLinkPath('other.md', 'docs/a.md')).toBe('docs/other.md')
  })

  it('resolves an explicit ./ link', () => {
    expect(resolvePreviewLinkPath('./other.md', 'docs/a.md')).toBe('docs/other.md')
  })

  it('resolves a ../ link', () => {
    expect(resolvePreviewLinkPath('../README.md', 'docs/guide/a.md')).toBe('docs/README.md')
  })

  it('resolves a root-relative link to a workspace-relative path (no leading slash)', () => {
    expect(resolvePreviewLinkPath('/docs/a.md', 'web/b.md')).toBe('docs/a.md')
  })

  it('drops the anchor fragment', () => {
    expect(resolvePreviewLinkPath('other.md#section', 'docs/a.md')).toBe('docs/other.md')
  })

  it('returns the current file for an anchor-only href', () => {
    expect(resolvePreviewLinkPath('#section', 'docs/a.md')).toBe('docs/a.md')
  })

  it('never escapes above the workspace root', () => {
    expect(resolvePreviewLinkPath('../../../etc/passwd', 'docs/a.md')).toBe('etc/passwd')
  })

  it('resolves against the root when the current file is at the root', () => {
    expect(resolvePreviewLinkPath('other.md', 'README.md')).toBe('other.md')
  })
})
