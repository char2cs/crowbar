import { editor as monacoEditor } from 'monaco-editor'
import type * as Monaco from 'monaco-editor'
import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react'

export interface CommentZoneSpec {
  /** Stable identity for this zone (thread id, or `composer:<line>`). */
  key: string
  /** 1-based Monaco model line; the zone renders BELOW this line. */
  afterModelLine: number
  /** React content rendered (via portal) into the zone. */
  node: ReactNode
}

interface ActiveZone {
  zoneId: string
  zone: Monaco.editor.IViewZone
  domNode: HTMLDivElement
  contentNode: HTMLDivElement
  observer: ResizeObserver
  afterModelLine: number
}

export interface CommentZonePortal {
  key: string
  contentNode: HTMLDivElement
  node: ReactNode
}

/**
 * Hosts inline review-comment threads as Monaco VIEW ZONES — the same
 * mechanism VS Code uses for PR comments. Each spec becomes a zone anchored
 * below its model line; React content is portaled into the zone's DOM node and
 * a ResizeObserver keeps the reserved zone height in sync as the content grows
 * (composer opens, replies arrive, markdown renders). A gutter "+" affordance
 * lets the user start a thread on any line.
 *
 * The hook owns the Monaco side (zones, height sync, the "+"); the caller owns
 * the data and renders the returned portals into its React tree.
 */
export function useDiffCommentZones(params: {
  editorRef: RefObject<Monaco.editor.IStandaloneCodeEditor | null>
  editorReady: boolean
  enabled: boolean
  zones: CommentZoneSpec[]
  onAddCommentAtLine?: (modelLine: number) => void
}): CommentZonePortal[] {
  const { editorRef, editorReady, enabled, zones, onAddCommentAtLine } = params
  const activeRef = useRef<Map<string, ActiveZone>>(new Map())
  const [portalKeys, setPortalKeys] = useState<string[]>([])

  // Structural signature (keys + anchor lines). Node-only changes do NOT churn
  // Monaco zones — the portals below re-render with fresh nodes every render.
  const signature = enabled ? zones.map((z) => `${z.key}@${z.afterModelLine}`).join('|') : ''

  // Reconcile view zones whenever the structure (or editor readiness) changes.
  useEffect(() => {
    const editor = editorRef.current
    if (!editorReady || !editor) return
    const active = activeRef.current
    const desired = enabled ? zones : []

    editor.changeViewZones((acc) => {
      // Remove zones no longer desired, or whose anchor line moved.
      for (const [key, az] of active) {
        const spec = desired.find((z) => z.key === key)
        if (!spec || spec.afterModelLine !== az.afterModelLine) {
          acc.removeZone(az.zoneId)
          az.observer.disconnect()
          active.delete(key)
        }
      }
      // Add new (or re-anchored) zones.
      for (const spec of desired) {
        if (active.has(spec.key)) continue
        const domNode = document.createElement('div')
        domNode.className = 'diff-comment-zone'
        const contentNode = document.createElement('div')
        domNode.appendChild(contentNode)
        // Start with a small reserved height; the observer corrects it once the
        // portal content has rendered and measured.
        const zone: Monaco.editor.IViewZone = {
          afterLineNumber: spec.afterModelLine,
          heightInPx: 80,
          domNode,
        }
        const zoneId = acc.addZone(zone)
        const observer = new ResizeObserver(() => {
          const height = contentNode.offsetHeight
          if (height > 0 && zone.heightInPx !== height) {
            zone.heightInPx = height
            editor.changeViewZones((a) => a.layoutZone(zoneId))
          }
        })
        observer.observe(contentNode)
        active.set(spec.key, {
          zoneId,
          zone,
          domNode,
          contentNode,
          observer,
          afterModelLine: spec.afterModelLine,
        })
      }
    })

    setPortalKeys(desired.filter((z) => active.has(z.key)).map((z) => z.key))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editorReady, signature, enabled])

  // Tear down all zones on unmount.
  useEffect(() => {
    return () => {
      const editor = editorRef.current
      const active = activeRef.current
      if (editor) {
        editor.changeViewZones((acc) => {
          for (const az of active.values()) acc.removeZone(az.zoneId)
        })
      }
      for (const az of active.values()) az.observer.disconnect()
      active.clear()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Gutter "+" affordance: hover shows a "+" in the glyph margin; clicking it
  // opens a composer on that line.
  useEffect(() => {
    const editor = editorRef.current
    if (!editorReady || !editor || !enabled || !onAddCommentAtLine) return
    editor.updateOptions({ glyphMargin: true })
    const hover = editor.createDecorationsCollection([])
    let lastLine = -1
    const setHover = (line: number) => {
      if (line === lastLine) return
      lastLine = line
      hover.set([
        {
          range: { startLineNumber: line, startColumn: 1, endLineNumber: line, endColumn: 1 },
          options: {
            glyphMarginClassName: 'diff-add-comment-glyph',
            glyphMarginHoverMessage: { value: 'Add a comment' },
          },
        },
      ])
    }
    const clearHover = () => {
      if (lastLine === -1) return
      lastLine = -1
      hover.clear()
    }
    const T = monacoEditor.MouseTargetType
    const moveSub = editor.onMouseMove((e) => {
      const line = e.target.position?.lineNumber
      const t = e.target.type
      const onCommentableRow =
        t === T.GUTTER_GLYPH_MARGIN ||
        t === T.GUTTER_LINE_NUMBERS ||
        t === T.CONTENT_TEXT ||
        t === T.CONTENT_EMPTY
      if (line && onCommentableRow) setHover(line)
      else clearHover()
    })
    const leaveSub = editor.onMouseLeave(clearHover)
    const downSub = editor.onMouseDown((e) => {
      if (e.target.type === T.GUTTER_GLYPH_MARGIN && e.target.position) {
        onAddCommentAtLine(e.target.position.lineNumber)
      }
    })
    return () => {
      moveSub.dispose()
      leaveSub.dispose()
      downSub.dispose()
      hover.clear()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editorReady, enabled, onAddCommentAtLine])

  const active = activeRef.current
  return portalKeys
    .map((key) => {
      const az = active.get(key)
      const spec = zones.find((z) => z.key === key)
      if (!az || !spec) return null
      return { key, contentNode: az.contentNode, node: spec.node }
    })
    .filter((p): p is CommentZonePortal => p !== null)
}
