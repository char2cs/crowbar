import type { ISearchOptions } from '@xterm/addon-search'
import type { Terminal } from '@xterm/xterm'
import { useCallback, useEffect, useState } from 'react'
import type { TerminalSearchOptions } from '../components/terminal-search'
import type { TerminalAddons } from './use-terminal-addons'

function searchOptions(options: TerminalSearchOptions): ISearchOptions {
  const rootStyles = getComputedStyle(document.documentElement)
  const selected = rootStyles.getPropertyValue('--color-selected').trim() || '#3b82f6'
  const accent = rootStyles.getPropertyValue('--color-accent').trim() || '#60a5fa'
  const border = rootStyles.getPropertyValue('--color-border').trim() || '#4b5563'
  return {
    caseSensitive: options.caseSensitive,
    wholeWord: options.wholeWord,
    regex: options.regex,
    decorations: {
      matchBackground: selected,
      matchBorder: border,
      matchOverviewRuler: selected,
      activeMatchBackground: accent,
      activeMatchBorder: border,
      activeMatchColorOverviewRuler: accent,
    },
  }
}

/** The terminal's find bar: whether it is open, its match count, and its actions. */
export function useTerminalSearch(terminal: Terminal | null, addons: TerminalAddons | null) {
  const [isVisible, setIsVisible] = useState(false)
  const [results, setResults] = useState({ current: 0, total: 0 })

  useEffect(() => {
    if (!addons) return
    const disposable = addons.searchAddon.onDidChangeResults(({ resultIndex, resultCount }) => {
      setResults({
        current: resultCount > 0 && resultIndex >= 0 ? resultIndex + 1 : 0,
        total: resultCount,
      })
    })
    return () => disposable.dispose()
  }, [addons])

  const clear = useCallback(() => {
    addons?.searchAddon.clearDecorations()
    terminal?.clearSelection()
    setResults({ current: 0, total: 0 })
  }, [addons, terminal])

  const open = useCallback(() => setIsVisible(true), [])
  const close = useCallback(() => {
    setIsVisible(false)
    clear()
    terminal?.focus()
  }, [clear, terminal])

  const onSearch = useCallback(
    (term: string, options: TerminalSearchOptions) => {
      if (!term || !addons) {
        clear()
        return
      }
      const found = addons.searchAddon.findNext(term, {
        ...searchOptions(options),
        incremental: true,
      })
      if (!found) setResults({ current: 0, total: 0 })
    },
    [addons, clear],
  )
  const onNext = useCallback(
    (term: string, options: TerminalSearchOptions) => {
      if (!term || !addons) return
      addons.searchAddon.findNext(term, searchOptions(options))
    },
    [addons],
  )
  const onPrevious = useCallback(
    (term: string, options: TerminalSearchOptions) => {
      if (!term || !addons) return
      addons.searchAddon.findPrevious(term, searchOptions(options))
    },
    [addons],
  )

  return {
    isVisible,
    open,
    close,
    barProps: {
      isVisible,
      onSearch,
      onNext,
      onPrevious,
      onClose: close,
      currentMatch: results.current,
      totalMatches: results.total,
    },
  }
}

export type TerminalSearchState = ReturnType<typeof useTerminalSearch>
