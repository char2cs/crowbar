import type { Terminal } from '@xterm/xterm'
import { useEffect, useRef, useState } from 'react'
import { useSettingsStore } from '@/features/settings/store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { resolveWorkspaceRootPath } from '@/lib/workspace/resolve-root-path'
import { toast } from '@/features/window/stores/toast-store'
import { injectLinkStyles, removeLinkStyles, type TerminalAddons } from './use-terminal-addons'
import { useTerminalTheme } from './use-terminal-theme'
import { fitToContainer } from './use-pty-size-sync'
import { mountXterm, type MountedXterm } from '../lib/mount-xterm'
import { workspaceRelativePath, type TerminalFileLinksOptions } from '../lib/terminal-file-links'
import { useTerminalStore } from '../stores/terminal-store'
import { resolveTerminalFont } from '../utils/resolve-font'

/**
 * File references open inside Crowbar: relative paths resolve against the
 * session cwd when known, else the workspace root, then are relativized back
 * onto the workspace root (the files API takes worktree-relative paths).
 */
function fileLinksFor(getSessionId: () => string): TerminalFileLinksOptions {
  return {
    getRoot: () => {
      const cwd = useTerminalStore.getState().getSession(getSessionId())?.currentDirectory
      if (cwd?.startsWith('/')) return cwd
      return resolveWorkspaceRootPath()
    },
    openFile: (absolutePath) => {
      const rel = workspaceRelativePath(absolutePath, resolveWorkspaceRootPath())
      if (!rel) {
        toast.error('Cannot open file', `${absolutePath} is outside the current workspace.`)
        return
      }
      const openHandler = useFileSystemStore.getState().handleFileOpen
      if (!openHandler) {
        toast.error('Cannot open file', 'No editor is available in this view.')
        return
      }
      void openHandler(rel).catch(() => {
        toast.error('Could not open file', absolutePath)
      })
    },
    onUnresolved: (candidateText) => {
      toast.error('Cannot open file', `Could not resolve ${candidateText} to a path.`)
    },
  }
}

interface UseXtermInstanceOptions {
  sessionId: string
  container: HTMLDivElement | null
  /** False until the view has been shown once: a never-seen tab builds nothing. */
  enabled: boolean
  /** Sends input the terminal itself produces (key overrides, drops, IME replacements). */
  write: (data: string, origin: string) => void
}

export interface XtermInstance {
  terminal: Terminal | null
  addons: TerminalAddons | null
}

/**
 * Builds this view's xterm once its container has a box, keeps its options in
 * step with the settings, and disposes it on unmount. It knows nothing about the
 * PTY: output and input are wired by useTerminalConnection, size by usePtySizeSync.
 */
export function useXtermInstance({
  sessionId,
  container,
  enabled,
  write,
}: UseXtermInstanceOptions): XtermInstance {
  const [instance, setInstance] = useState<XtermInstance>({ terminal: null, addons: null })
  const writeRef = useRef(write)
  const sessionIdRef = useRef(sessionId)
  useEffect(() => {
    writeRef.current = write
    sessionIdRef.current = sessionId
  }, [write, sessionId])

  const fontFamily = useSettingsStore((state) => state.settings.terminalFontFamily)
  const baseFontSize = useSettingsStore((state) => state.settings.terminalFontSize)
  const lineHeight = useSettingsStore((state) => state.settings.terminalLineHeight)
  const baseLetterSpacing = useSettingsStore((state) => state.settings.terminalLetterSpacing)
  const scrollback = useSettingsStore((state) => state.settings.terminalScrollback)
  const cursorStyle = useSettingsStore((state) => state.settings.terminalCursorStyle)
  const cursorBlink = useSettingsStore((state) => state.settings.terminalCursorBlink)
  const baseCursorWidth = useSettingsStore((state) => state.settings.terminalCursorWidth)
  const zoomLevel = useZoomStore.use.terminalZoomLevel()
  const { getTerminalTheme } = useTerminalTheme()
  const fontSize = Math.round(baseFontSize * zoomLevel * 10) / 10
  const letterSpacing = baseLetterSpacing * zoomLevel
  const cursorWidth = Math.max(1, Math.round(baseCursorWidth * zoomLevel))
  const settings = {
    terminalFontFamily: fontFamily,
    terminalLineHeight: lineHeight,
    terminalScrollback: scrollback,
    terminalCursorStyle: cursorStyle,
    terminalCursorBlink: cursorBlink,
  }

  // The options a new xterm is born with, read through a ref so the build effect
  // runs once per container rather than once per settings change.
  const optionsRef = useRef({ settings, fontSize, letterSpacing, cursorWidth, getTerminalTheme })
  optionsRef.current = { settings, fontSize, letterSpacing, cursorWidth, getTerminalTheme }

  // Build — once the container has a box. A container that is still 0×0 (laid out
  // later, or display:none) is watched with a ResizeObserver instead of polled.
  const [hasBox, setHasBox] = useState(false)
  useEffect(() => {
    if (!enabled || !container || hasBox) return
    const measure = () => {
      const rect = container.getBoundingClientRect()
      if (rect.width > 0 && rect.height > 0) setHasBox(true)
    }
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(measure)
    observer.observe(container)
    return () => observer.disconnect()
  }, [enabled, container, hasBox])

  useEffect(() => {
    if (!hasBox || !container) return
    let disposed = false
    let mounted: MountedXterm | null = null
    const o = optionsRef.current

    void resolveTerminalFont(o.settings.terminalFontFamily, o.fontSize)
      .then((fontFamily) => {
        if (disposed) return
        mounted = mountXterm(
          container,
          {
            fontFamily,
            fontSize: o.fontSize,
            lineHeight: o.settings.terminalLineHeight,
            letterSpacing: o.letterSpacing,
            cursorBlink: o.settings.terminalCursorBlink,
            cursorStyle: o.settings.terminalCursorStyle,
            cursorWidth: o.cursorWidth,
            allowTransparency: true,
            theme: o.getTerminalTheme(),
            scrollback: o.settings.terminalScrollback,
            convertEol: false,
            macOptionIsMeta: true,
            rightClickSelectsWord: false,
          },
          {
            write: (data, origin) => writeRef.current(data, origin),
            fileLinks: fileLinksFor(() => sessionIdRef.current),
          },
        )
        fitToContainer(mounted.addons.fitAddon, container)
        setInstance({ terminal: mounted.terminal, addons: mounted.addons })
      })
      .catch((error: unknown) => {
        console.error('Failed to initialize terminal:', error)
      })

    return () => {
      disposed = true
      mounted?.dispose()
      mounted = null
      setInstance({ terminal: null, addons: null })
    }
  }, [hasBox, container])

  const { terminal, addons } = instance

  // Link colouring is scoped to the container, whose id follows the session — so
  // an attach-only swap re-scopes it along with the PTY.
  useEffect(() => {
    if (!terminal || !container) return
    injectLinkStyles(sessionId, container.id || `terminal-${sessionId}`)
    return () => removeLinkStyles(sessionId)
  }, [terminal, container, sessionId])

  // Keep a live xterm's options in step with the settings.
  useEffect(() => {
    if (!terminal || !addons) return
    let cancelled = false
    void resolveTerminalFont(settings.terminalFontFamily, fontSize).then((fontFamily) => {
      if (cancelled) return
      terminal.options.fontFamily = fontFamily
      terminal.options.fontSize = fontSize
      terminal.options.lineHeight = settings.terminalLineHeight
      terminal.options.letterSpacing = letterSpacing
      terminal.options.scrollback = settings.terminalScrollback
      terminal.options.cursorBlink = settings.terminalCursorBlink
      terminal.options.cursorStyle = settings.terminalCursorStyle
      terminal.options.cursorWidth = cursorWidth
      fitToContainer(addons.fitAddon, container)
    })
    return () => {
      cancelled = true
    }
  }, [
    terminal,
    addons,
    container,
    settings.terminalFontFamily,
    settings.terminalLineHeight,
    settings.terminalScrollback,
    settings.terminalCursorBlink,
    settings.terminalCursorStyle,
    fontSize,
    letterSpacing,
    cursorWidth,
  ])

  return instance
}
