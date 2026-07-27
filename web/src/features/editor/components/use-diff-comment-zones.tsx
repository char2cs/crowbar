// See the comment in `monaco-diff-editor.tsx`: `editor.api` is the same real
// editor singleton as the bare 'monaco-editor' specifier, without eagerly
// bundling all built-in language contributions.
import { editor as monacoEditor } from 'monaco-editor/esm/vs/editor/editor.api.js'
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
        // react-doctor-disable-next-line js-index-maps -- `desired` is the diff's review-comment zones (bounded by how many comments a user has added, typically a handful); building a Map here would cost readability for no measurable gain.
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
            // Defer the relayout out of the observation callback (avoids the
            // benign WebKit "ResizeObserver loop" warning + a sync reflow), and
            // skip it if the editor was disposed between frames.
            requestAnimationFrame(() => {
              if (editorRef.current !== editor) return
              editor.changeViewZones((a) => a.layoutZone(zoneId))
            })
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

    // react-doctor-disable-next-line js-combine-iterations -- `desired` is the diff's review-comment zones (bounded by how many comments a user has added, typically a handful); a single-pass rewrite here would cost readability for no measurable gain.
    setPortalKeys(desired.filter((z) => active.has(z.key)).map((z) => z.key))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editorReady, signature, enabled])

  // Tear down all zones on unmount. FP: this is a mount-only teardown whose
  // cleanup must read editorRef.current *fresh* (the diff editor is recreated
  // across the component's life); the empty deps are correct.
  // react-doctor-disable-next-line exhaustive-deps
  useEffect(() => {
    // activeRef holds a stable Map (only mutated, never reassigned) — capture it
    // in the effect body per exhaustive-deps. editorRef is read *fresh* in the
    // cleanup on purpose: the diff editor can be disposed/recreated during the
    // component's life, so at unmount we must target whatever editor is current
    // (capturing it here could pin a null/stale editor).
    const active = activeRef.current
    return () => {
      const editor = editorRef.current
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

  // When the Monaco editor is disposed (editorReady → false, e.g. the diff
  // editor is recreated for a new buffer), the zone ids in `active` belong to
  // the dead editor. Drop them so the recreated editor re-adds its zones
  // instead of skipping them as "already present", and stop orphaned observers.
  // Expressed as the live editor's CLEANUP rather than a reaction to
  // editorReady going false: same trigger, but it also covers unmount, and it
  // reads as what it is — releasing the zones this editor owned.
  useEffect(() => {
    if (!editorReady) return
    const active = activeRef.current
    return () => {
      for (const az of active.values()) az.observer.disconnect()
      active.clear()
      setPortalKeys([])
    }
  }, [editorReady])

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
