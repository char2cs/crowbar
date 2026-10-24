import { useEffect, useRef, useState } from 'react'
import { readDocument, type DocumentRef } from '@/lib/api/documents'
import { siblingRef } from '@/features/editor/documents/document-ref'
import { usePreviewSource } from '@/features/editor/documents/use-preview-source'
import { mimeForPath, toDataUrl } from '@/features/editor/lib/asset-data-url'
import { buildHtmlPreviewDocument, withHostPreviewPolicy } from './html-preview-document'

// No `allow-same-origin`: combined with `allow-scripts` it lets the framed
// document reach the parent Crowbar origin and remove its own sandbox. A host
// document is any file on disk, so it gets no scripts at all.
const WORKSPACE_SANDBOX = 'allow-scripts allow-forms allow-popups allow-modals'
const HOST_SANDBOX = ''

/** The asset `reference` names beside `doc`; a host document reaches only its own subtree. */
function assetRef(doc: DocumentRef, reference: string): DocumentRef | null {
  const ref = siblingRef(doc, reference)
  if (doc.authority === 'workspace') return ref
  const dir = doc.path.slice(0, doc.path.lastIndexOf('/') + 1)
  return ref.path.startsWith(dir) ? ref : null
}

export function HtmlPreview({ bufferId }: { bufferId?: string }) {
  const source = usePreviewSource(bufferId)
  const { doc } = source
  const [iframeContent, setIframeContent] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)

  // Local assets are read as documents beside the file and inlined; the latest
  // edit wins over a slower earlier build.
  useEffect(() => {
    let cancelled = false
    const loadAsset = async (reference: string) => {
      const ref = doc ? assetRef(doc, reference) : null
      const mime = ref ? mimeForPath(ref.path) : null
      if (!ref || !mime) return null
      return toDataUrl(mime, (await readDocument(ref)).text)
    }
    void buildHtmlPreviewDocument(source.content, loadAsset).then((html) => {
      if (cancelled) return
      setIframeContent(doc?.authority === 'host' ? withHostPreviewPolicy(html) : html)
    })
    return () => {
      cancelled = true
    }
  }, [source.content, doc])

  if (!source.shown) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        No active buffer
      </div>
    )
  }

  return (
    <div ref={containerRef} className="html-preview h-full w-full bg-white">
      <iframe
        title="HTML Preview"
        srcDoc={iframeContent}
        className="h-full w-full border-none"
        sandbox={doc?.authority === 'host' ? HOST_SANDBOX : WORKSPACE_SANDBOX}
      />
    </div>
  )
}
