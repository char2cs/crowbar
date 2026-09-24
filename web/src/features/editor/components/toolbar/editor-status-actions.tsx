import { extensionRegistry } from '@/extensions/registry/extension-registry'
import { Check, SlidersHorizontal } from '@phosphor-icons/react'
import { useCallback, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useCommandShortcut } from '@/features/keymaps/hooks/use-command-shortcut'
import { setSyntaxHighlightingFilePath } from '@/features/editor/extensions/builtin/syntax-highlighting'
import { isMarkdownPath } from '@/features/editor/markdown/plate/is-markdown-path'
import { MarkdownViewToggle } from '@/features/editor/markdown/plate/markdown-view-toggle'
import type { Position } from '@/features/editor/types/editor'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import {
  getAllLanguages,
  getLanguageDisplayName,
  getLanguageIdFromPath,
} from '@/features/editor/utils/language-id'
import { useSettingsStore } from '@/features/settings/store'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Dropdown, dropdownItemClassName } from '@/components/ui/dropdown'
import Keybinding from '@/components/ui/keybinding'
import { cn } from '@/utils/cn'
import { LspStatusMenu } from './lsp-status-menu'

const statusChipClass =
  'ui-font inline-flex h-5 items-center self-center rounded-md border border-transparent px-1.5 ui-text-xs leading-none text-muted-foreground transition-colors hover:bg-muted hover:text-foreground'

const menuTriggerClass = cn(buttonVariants({ variant: 'ghost' }), 'rounded text-muted-foreground')

const menuItemClass =
  'ui-font flex w-full items-center justify-between gap-3 rounded-lg px-2.5 py-1.5 text-left ui-text-xs text-foreground transition-colors hover:bg-muted'

const menuItemDisabledClass = 'cursor-not-allowed opacity-50 hover:bg-transparent'

function getLanguageDisplayNameOrNull(languageId: string | null) {
  if (!languageId) return null
  return getLanguageDisplayName(languageId)
}

interface EditorStatusActionsProps {
  bufferId?: string
  editorViewKey?: string | null
}

function CursorPositionChip({ editorViewKey }: { editorViewKey?: string | null }) {
  const activeEditorViewKey = useEditorStateStore.use.activeEditorViewKey()
  const cursorPosition = useEditorStateStore.use.cursorPosition()
  const displayedCursorPosition = useMemo<Position>(() => {
    if (!editorViewKey || activeEditorViewKey === editorViewKey) {
      return cursorPosition
    }

    const cachedCursor = useEditorStateStore.getState().actions.getCachedPosition(editorViewKey)
    return cachedCursor ?? { line: 0, column: 0, offset: 0 }
  }, [activeEditorViewKey, cursorPosition, editorViewKey])

  return (
    <span className={statusChipClass}>
      {displayedCursorPosition.line + 1}:{displayedCursorPosition.column + 1}
    </span>
  )
}

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive toolbar — cursor/language/LSP-status controls share the active-buffer selectors and command shortcuts; touched this program (getStatusConfig hoisted) with no further seam.
export function EditorStatusActions({ bufferId, editorViewKey }: EditorStatusActionsProps = {}) {
  const resolvedBufferId = useStore(
    windowPaneStore,
    (state) => bufferId ?? state.panes[state.activePaneId]?.activeEditorTabId ?? null,
  )
  const settings = useSettingsStore((s) => s.settings)
  const updateSetting = useSettingsStore((s) => s.updateSetting)
  const minimapShortcut = useCommandShortcut('workbench.toggleMinimap')
  const [isViewMenuOpen, setIsViewMenuOpen] = useState(false)
  const [isLanguageOpen, setIsLanguageOpen] = useState(false)
  const [languageSearch, setLanguageSearch] = useState('')
  const viewButtonRef = useRef<HTMLButtonElement>(null)
  const languageButtonRef = useRef<HTMLButtonElement>(null)
  const languageSearchRef = useRef<HTMLInputElement>(null)

  const activeBuffer = useStore(
    windowPaneStore,
    useShallow((state) => {
      const buffer = resolvedBufferId
        ? state.buffers.find((candidate) => candidate.id === resolvedBufferId)
        : null
      return buffer
        ? {
            id: buffer.id,
            path: buffer.path,
            type: buffer.type,
            workspaceId: buffer.workspaceId,
            languageOverride: buffer.type === 'editor' ? buffer.languageOverride : undefined,
          }
        : null
    }),
  )
  const currentFileLanguageId =
    activeBuffer?.type === 'editor' && activeBuffer.languageOverride
      ? activeBuffer.languageOverride
      : activeBuffer?.path
        ? getLanguageIdFromPath(activeBuffer.path) ||
          extensionRegistry.getLanguageId(activeBuffer.path)
        : null
  const currentFileDisplayName = getLanguageDisplayNameOrNull(currentFileLanguageId)

  const allLanguages = useMemo(() => getAllLanguages(), [])

  const filteredLanguages = useMemo(() => {
    if (!languageSearch) return allLanguages
    const query = languageSearch.toLowerCase()
    return allLanguages.filter(
      (lang) =>
        lang.displayName.toLowerCase().includes(query) || lang.id.toLowerCase().includes(query),
    )
  }, [allLanguages, languageSearch])

  const handleLanguageChange = useCallback(
    async (languageId: string) => {
      if (!activeBuffer || !resolvedBufferId || activeBuffer.type !== 'editor') return
      if (languageId === currentFileLanguageId) {
        setIsLanguageOpen(false)
        return
      }

      windowPaneStore.setState((state) => ({
        buffers: state.buffers.map((b) =>
          b.id === resolvedBufferId && b.type === 'editor'
            ? { ...b, languageOverride: languageId }
            : b,
        ),
      }))

      // The editor re-opens the document with the new language id on its
      // own (its LSP document lifecycle is keyed by language).
      if (activeBuffer.path) {
        await setSyntaxHighlightingFilePath(activeBuffer.path)
      }

      setIsLanguageOpen(false)
      setLanguageSearch('')
    },
    [activeBuffer, resolvedBufferId, currentFileLanguageId],
  )

  const displayOptions = [
    {
      id: 'breadcrumbs',
      label: 'Breadcrumbs',
      checked: settings.coreFeatures.breadcrumbs,
      shortcut: null,
      onToggle: () =>
        updateSetting('coreFeatures', {
          ...settings.coreFeatures,
          breadcrumbs: !settings.coreFeatures.breadcrumbs,
        }),
    },
    {
      id: 'minimap',
      label: 'Minimap',
      checked: settings.showMinimap,
      shortcut: minimapShortcut,
      onToggle: () => updateSetting('showMinimap', !settings.showMinimap),
    },
    {
      id: 'line-numbers',
      label: 'Line Numbers',
      checked: settings.lineNumbers,
      shortcut: null,
      onToggle: () => updateSetting('lineNumbers', !settings.lineNumbers),
      disabled: false,
    },
    {
      id: 'word-wrap',
      label: 'Word Wrap',
      checked: settings.wordWrap,
      shortcut: null,
      onToggle: () => updateSetting('wordWrap', !settings.wordWrap),
      disabled: false,
    },
    {
      id: 'parameter-hints',
      label: 'Parameter Hints',
      checked: settings.parameterHints,
      shortcut: null,
      onToggle: () => updateSetting('parameterHints', !settings.parameterHints),
      disabled: false,
    },
    {
      id: 'auto-completion',
      label: 'Auto Completion',
      checked: settings.autoCompletion,
      shortcut: null,
      onToggle: () => updateSetting('autoCompletion', !settings.autoCompletion),
      disabled: false,
    },
  ]

  return (
    <>
      <CursorPositionChip editorViewKey={editorViewKey} />

      {activeBuffer?.type === 'editor' && (
        <div className="relative flex h-5 items-center self-center">
          <Button
            ref={languageButtonRef}
            type="button"
            onClick={() => {
              setIsLanguageOpen((open) => !open)
              setLanguageSearch('')
            }}
            variant="ghost"
            compact
            className={cn(
              statusChipClass,
              'min-w-0 cursor-pointer',
              isLanguageOpen && 'bg-muted text-foreground',
            )}
            aria-expanded={isLanguageOpen}
            aria-haspopup="listbox"
            tooltip="Select language mode"
            tooltipSide="bottom"
          >
            {currentFileDisplayName || 'Plain Text'}
          </Button>
          <Dropdown
            isOpen={isLanguageOpen}
            anchorRef={languageButtonRef}
            anchorSide="bottom"
            anchorAlign="end"
            onClose={() => {
              setIsLanguageOpen(false)
              setLanguageSearch('')
            }}
            className="w-[220px] overflow-hidden rounded-lg p-1.5"
          >
            <div className="px-1.5 pb-1.5">
              <input
                ref={languageSearchRef}
                type="text"
                value={languageSearch}
                onChange={(e) => setLanguageSearch(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Escape') {
                    setIsLanguageOpen(false)
                    setLanguageSearch('')
                  }
                }}
                placeholder="Search languages..."
                className="ui-font w-full rounded-md border border-border/70 bg-background px-2 py-1 ui-text-xs text-foreground outline-none placeholder:text-muted-foreground/50 focus:border-secondary/50"
                autoFocus
                aria-label="Search languages"
              />
            </div>
            <div className="max-h-[200px] overflow-y-auto">
              {filteredLanguages.map((lang) => (
                <Button
                  key={lang.id}
                  type="button"
                  onClick={() => void handleLanguageChange(lang.id)}
                  variant="ghost"
                  compact
                  className={dropdownItemClassName(
                    cn('justify-between', lang.id === currentFileLanguageId && 'text-secondary'),
                  )}
                  role="option"
                  aria-selected={lang.id === currentFileLanguageId}
                >
                  <span className="truncate">{lang.displayName}</span>
                  {lang.id === currentFileLanguageId && (
                    <Check className="shrink-0 text-secondary" />
                  )}
                </Button>
              ))}
              {filteredLanguages.length === 0 && (
                <div className="px-2.5 py-2 text-center text-muted-foreground ui-text-xs">
                  No languages found
                </div>
              )}
            </div>
          </Dropdown>
        </div>
      )}

      {activeBuffer?.type === 'editor' &&
        activeBuffer.path &&
        isMarkdownPath(activeBuffer.path) && <MarkdownViewToggle bufferId={activeBuffer.id} />}

      {activeBuffer?.type === 'editor' && activeBuffer.path && (
        <LspStatusMenu workspaceId={activeBuffer.workspaceId} path={activeBuffer.path} />
      )}

      <div className="relative flex items-center self-center">
        <Button
          ref={viewButtonRef}
          type="button"
          onClick={() => setIsViewMenuOpen((open) => !open)}
          variant="ghost"
          compact
          className={cn(
            menuTriggerClass,
            isViewMenuOpen && 'border-border/60 bg-muted/80 text-foreground',
          )}
          tooltip="Editor preferences"
          tooltipSide="bottom"
        >
          <span className="flex size-full items-center justify-center">
            <SlidersHorizontal />
          </span>
        </Button>
        <Dropdown
          isOpen={isViewMenuOpen}
          anchorRef={viewButtonRef}
          anchorSide="bottom"
          anchorAlign="end"
          onClose={() => setIsViewMenuOpen(false)}
          className="w-[220px] overflow-hidden rounded-lg p-1.5"
        >
          <div className="space-y-0.5">
            {displayOptions.slice(0, 2).map((option) => (
              <Button
                key={option.id}
                type="button"
                onClick={() => !option.disabled && void option.onToggle()}
                variant="ghost"
                compact
                className={cn(menuItemClass, option.disabled && menuItemDisabledClass)}
                disabled={option.disabled}
              >
                <span>{option.label}</span>
                <span className="flex items-center gap-2">
                  {option.shortcut ? (
                    <Keybinding binding={option.shortcut} className="shrink-0" />
                  ) : null}
                  <span className="flex size-4 items-center justify-center">
                    {option.checked ? <Check className="text-secondary" /> : null}
                  </span>
                </span>
              </Button>
            ))}
            <div className="my-1 border-t border-border/70" />
            {displayOptions.slice(2, 6).map((option) => (
              <Button
                key={option.id}
                type="button"
                onClick={() => !option.disabled && void option.onToggle()}
                variant="ghost"
                compact
                className={cn(menuItemClass, option.disabled && menuItemDisabledClass)}
                disabled={option.disabled}
              >
                <span>{option.label}</span>
                <span className="flex size-4 items-center justify-center">
                  {option.checked ? <Check className="text-secondary" /> : null}
                </span>
              </Button>
            ))}
            <div className="my-1 border-t border-border/70" />
            {displayOptions.slice(6).map((option) => (
              <Button
                key={option.id}
                type="button"
                onClick={() => !option.disabled && void option.onToggle()}
                variant="ghost"
                compact
                className={cn(menuItemClass, option.disabled && menuItemDisabledClass)}
                disabled={option.disabled}
              >
                <span>{option.label}</span>
                <span className="flex size-4 items-center justify-center">
                  {option.checked ? <Check className="text-secondary" /> : null}
                </span>
              </Button>
            ))}
          </div>
        </Dropdown>
      </div>
    </>
  )
}
