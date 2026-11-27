import { createElement } from 'react'
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, it, expect } from 'vitest'
import { settingsSearchIndex } from '@/features/settings/config/search-index'
import { SETTINGS_TAB_ITEMS } from '@/features/settings/components/settings-tab-items'
import { TooltipProvider } from '@/components/ui/tooltip'
import { scoreSearchQuery } from '@/utils/search-match'
import { AppearanceSettings } from '@/features/settings/components/tabs/appearance-settings'
import { EditorSettings } from '@/features/settings/components/tabs/editor-settings'
import { FileTreeSettings } from '@/features/settings/components/tabs/file-tree-settings'
import { GitSettings } from '@/features/settings/components/tabs/git-settings'
import { ProvidersSettings } from '@/features/settings/components/tabs/providers-settings'
import { BrowserSettings } from '@/features/settings/components/tabs/browser-settings'
import { TerminalSettings } from '@/features/settings/components/tabs/terminal-settings'
import type { SettingsTab } from '@/features/window/stores/ui-state-store'

const TAB_COMPONENTS: Partial<Record<SettingsTab, () => React.ReactNode>> = {
  appearance: AppearanceSettings,
  browser: BrowserSettings,
  editor: EditorSettings,
  'file-explorer': FileTreeSettings,
  git: GitSettings,
  providers: ProvidersSettings,
  terminal: TerminalSettings,
}

interface RenderedTab {
  sections: Set<string>
  text: string
}

function renderTab(tab: SettingsTab): RenderedTab {
  const Component = TAB_COMPONENTS[tab]
  if (!Component) throw new Error(`no component mapped for settings tab "${tab}"`)
  const { container } = render(createElement(TooltipProvider, null, createElement(Component)))
  const rendered = {
    sections: new Set(
      [...container.querySelectorAll('[data-settings-section]')].map(
        (el) => el.getAttribute('data-settings-section') ?? '',
      ),
    ),
    text: container.textContent ?? '',
  }
  cleanup()
  return rendered
}

function tabsMatching(query: string): Set<SettingsTab> {
  const q = query.trim().toLowerCase()
  const tabs = new Set<SettingsTab>()
  for (const record of settingsSearchIndex) {
    const score = scoreSearchQuery(q, [
      { value: record.label, weight: 11 },
      { value: record.description, weight: 1 },
      { value: record.section, weight: 1 },
      ...(record.keywords ?? []).map((keyword) => ({ value: keyword, weight: 6 })),
    ])
    if (score > 0) tabs.add(record.tab)
  }
  return tabs
}

afterEach(cleanup)

describe('settings search index', () => {
  it('has unique search-record ids', () => {
    const ids = settingsSearchIndex.map((r) => r.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  describe('the Agents tab is findable', () => {
    it.each(['provider', 'providers', 'agents', 'claude', 'codex', 'cli', 'agent', 'priority'])(
      '"%s" narrows to the Agents tab',
      (query) => {
        expect([...tabsMatching(query)]).toContain('providers')
      },
    )

    it('covers every control the tab exposes', () => {
      const records = settingsSearchIndex.filter((r) => r.tab === 'providers')
      expect(records.map((r) => r.id).sort()).toEqual([
        'agents-chat-default-presentation',
        'providers-enabled',
        'providers-priority-order',
        'providers-tools',
      ])
    })

    it.each(['tools', 'mcp'])('"%s" narrows to the Agents tab', (query) => {
      expect([...tabsMatching(query)]).toContain('providers')
    })

    it.each(['chat', 'terminal', 'presentation', 'surface'])(
      '"%s" reaches the chat landing-surface toggle',
      (query) => {
        expect([...tabsMatching(query)]).toContain('providers')
      },
    )
  })

  it('every record points at a real settings tab', () => {
    const tabs = new Set<string>([...SETTINGS_TAB_ITEMS.map((t) => t.id), 'developer'])
    const strays = settingsSearchIndex.filter((r) => !tabs.has(r.tab)).map((r) => r.id)
    expect(strays).toEqual([])
  })

  it('every record lands on something its tab actually renders', () => {
    const byTab = new Map<SettingsTab, RenderedTab>()
    const orphans = settingsSearchIndex
      .filter((record) => {
        if (!byTab.has(record.tab)) byTab.set(record.tab, renderTab(record.tab))
        const tab = byTab.get(record.tab)!
        return !tab.sections.has(record.section) && !tab.text.includes(record.label)
      })
      .map((record) => `${record.id} → ${record.tab}/${record.section}/${record.label}`)

    expect(orphans).toEqual([])
  })
})
