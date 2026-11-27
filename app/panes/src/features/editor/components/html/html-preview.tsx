import { useEffect, useRef, useState } from 'react'
import { readDocument, type DocumentRef } from '@/lib/api/documents'
import { siblingRef } from '@/features/editor/documents/document-ref'
import { usePreviewSource } from '@/features/editor/documents/use-preview-source'
import { mimeForPath, toDataUrl } from '@/features/editor/lib/asset-data-url'
import { buildHostPreviewDocument, buildHtmlPreviewDocument } from './html-preview-document'

const WORKSPACE_SANDBOX = 'allow-scripts allow-forms allow-popups allow-modals'
const HOST_SANDBOX = ''

export function HtmlPreview({ bufferId }: { bufferId?: string }) {
  const { shown, doc, content } = usePreviewSource(bufferId)
  if (!shown) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        No active buffer
      </div>
    )
  }
  return <HtmlPreviewView doc={doc} content={content} />
}

export function HtmlPreviewView({ doc, content }: { doc: DocumentRef | null; content: string }) {
  const [iframeContent, setIframeContent] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let cancelled = false
    const read = async (ref: DocumentRef) => (await readDocument(ref)).text
    const loadAsset = async (reference: string) => {
      const ref = doc ? siblingRef(doc, reference) : null
      const mime = ref ? mimeForPath(ref.path) : null
      if (!ref || !mime) return null
      return toDataUrl(mime, await read(ref))
    }
    const built =
      doc?.authority === 'host'
        ? buildHostPreviewDocument(content, doc, read)
        : buildHtmlPreviewDocument(content, loadAsset)
    void built.then((html) => {
      if (!cancelled) setIframeContent(html)
    })
    return () => {
      cancelled = true
    }
  }, [content, doc])

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
