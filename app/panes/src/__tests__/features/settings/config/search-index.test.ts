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
import { TerminalSettings } from '@/features/settings/components/tabs/terminal-settings'
import type { SettingsTab } from '@/features/window/stores/ui-state-store'

// Every tab that owns search records, and the component the settings dialog
// renders for it (settings-dialog.tsx's renderTabContent). A record for a tab
// that is missing here throws rather than passing silently, so adding records
// for a new tab forces that tab into this map instead of escaping the guard.
const TAB_COMPONENTS: Partial<Record<SettingsTab, () => React.ReactNode>> = {
  appearance: AppearanceSettings,
  editor: EditorSettings,
  'file-explorer': FileTreeSettings,
  git: GitSettings,
  providers: ProvidersSettings,
  terminal: TerminalSettings,
}

interface RenderedTab {
  /** Section titles the tab renders — `<Section title>` publishes each as
   *  data-settings-section, and a search hit lands the user in one of them. */
  sections: Set<string>
  /** Everything the tab renders as text, i.e. every control's own label. */
  text: string
}

function renderTab(tab: SettingsTab): RenderedTab {
  const Component = TAB_COMPONENTS[tab]
  if (!Component) throw new Error(`no component mapped for settings tab "${tab}"`)
  // Same provider main.tsx wraps the app in — a row with a tooltip throws
  // without it, exactly as it would in the real dialog.
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

/** The tabs a query narrows the rail down to — exactly what the settings store's
 *  runSearch scores, and what filterSettingsTabsBySearch then filters with. */
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

  // The tab shipped with NO records at all, so searching for the very thing it is
  // called collapsed the rail to "No matching settings" — the search hid the one
  // tab that matched.
  describe('the Agents tab is findable', () => {
    // The tab reads "Agents" now (spec §11) and its labels follow, but the OLD
    // word still has to find it: `provider` is what the rest of the product and
    // every existing habit says, so it stays a keyword on every record.
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

    // The one control whose own name is deliberately NOT the term of art: the row
    // says "Tools", so someone who knows it as MCP can only arrive by keyword.
    it.each(['tools', 'mcp'])('"%s" narrows to the Agents tab', (query) => {
      expect([...tabsMatching(query)]).toContain('providers')
    })

    // The landing-surface toggle lives on this tab but is about Chat, so it is
    // reachable by the surface's words as well as the tab's.
    it.each(['chat', 'terminal', 'presentation', 'surface'])(
      '"%s" reaches the chat landing-surface toggle',
      (query) => {
        expect([...tabsMatching(query)]).toContain('providers')
      },
    )
  })

  it('every record points at a real settings tab', () => {
    // `developer` is DEV-gated out of the rail but is a real tab.
    const tabs = new Set<string>([...SETTINGS_TAB_ITEMS.map((t) => t.id), 'developer'])
    const strays = settingsSearchIndex.filter((r) => !tabs.has(r.tab)).map((r) => r.id)
    expect(strays).toEqual([])
  })

  // THE DRIFT GUARD. A search record is a promise that a control exists; delete
  // the control and the record goes on making it — except now the query narrows
  // the whole tab rail down to a tab that visibly does not contain what was
  // searched for. Which is precisely what this branch did: it replaced
  // CoreFeaturesState with `{ breadcrumbs }` and left a `features-git` record
  // advertising a "Git Integration" toggle, in an "Integration" section that no
  // longer exists.
  //
  // A record must therefore land on SOMETHING its own tab really renders: the
  // section it names, or its own label as text. Either is enough on purpose —
  // several records deliberately carry a search-friendlier label than the row
  // wears ("Terminal Font Family" for a row labelled "Font Family"), and several
  // sit in a section that was later consolidated. A record matching NEITHER is
  // pointing at nothing, which is the shape a deleted control leaves behind.
  //
  // KNOWN LIMIT: a control deleted out of a section that still holds other rows
  // slips through this. Nothing short of an explicit per-record control anchor
  // catches that, and the three such orphans found by hand while writing this
  // (editor-engine, editor-custom-editor-command, appearance-window-transparency
  // — settings that are still read at runtime but have never had a rendered
  // control) were deleted from the index in the same pass.
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
