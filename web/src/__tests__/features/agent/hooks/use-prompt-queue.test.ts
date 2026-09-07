import { describe, expect, it } from 'vitest'
import { hasPendingImageUpload } from '@/features/agent/hooks/use-prompt-queue'

// REGRESSION, reported live: "photos attachments are not loaded instantly...
// let's not wait for them." Once a photo shows an instant local preview
// (insertPendingImageInto/settlePendingImageInto, chat-markdown-editor.tsx)
// instead of waiting for its upload, sending has to wait for the SWAP
// instead — a `blob:` url is meaningless outside this browser session, and
// the agent could never fetch it. `enqueue` (usePromptQueue) refuses to
// queue a draft that still contains one; this is that check's own pure
// predicate, unit-tested directly the same way `isPromptTextWithinLimit`
// (prompt-queue-persistence.ts) already is.
describe('hasPendingImageUpload', () => {
  it('is true for a draft containing a pending image placeholder', () => {
    expect(hasPendingImageUpload('check this out ![photo.png](blob:local-preview-id) thanks')).toBe(
      true,
    )
  })

  it('is false for a draft with no images at all', () => {
    expect(hasPendingImageUpload('just some plain text')).toBe(false)
  })

  it('is false for a draft whose image already resolved to a real ref', () => {
    expect(hasPendingImageUpload('![photo.png](chats/c1/attachments/x-photo.png)')).toBe(false)
  })

  // The check is markdown-image-shaped (`![alt](blob:...)`), not a bare
  // substring match — plain text that happens to mention "blob" must not
  // false-positive and block sending.
  it('is false for text that merely mentions the word "blob"', () => {
    expect(hasPendingImageUpload('a blob: of clay, not an attachment')).toBe(false)
  })

  it('is true for multiple images even when only one is still pending', () => {
    const draft =
      '![done](chats/c1/attachments/a.png) and ![pending](blob:local-preview-id)'
    expect(hasPendingImageUpload(draft)).toBe(true)
  })
})
