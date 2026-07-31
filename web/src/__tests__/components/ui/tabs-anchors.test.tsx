import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Tabs, TabsList, TabsPanel, TabsTab } from '@/components/ui/tabs'

/**
 * Every anchor `native/mapping/tabs.md` names must be reachable by
 * `data-oracle-id` alone — the extractor never looks at anything else, so an
 * untagged element is invisible to the oracle.
 *
 * The tab's and the panel's ids are **derived from their own `value`**, and that
 * is what these assertions are really about: `ANCHORS.md` v1.8 says a snapshot
 * carries each anchor at most once, a live `tabs` root holds three to six tabs,
 * and no live call site can be edited to name them. So the uniqueness has to come
 * out of the primitive, and this is where that is checked rather than assumed.
 */
describe('tabs oracle anchors', () => {
  it('tags every slot, and gives each tab an id built from its own value', () => {
    const { container } = render(
      <Tabs defaultValue="workspaces">
        <TabsList variant="default">
          <TabsTab value="workspaces">Workspaces</TabsTab>
          <TabsTab value="chats">Chats</TabsTab>
          <TabsTab value="files">Files</TabsTab>
        </TabsList>
        <TabsPanel value="workspaces">panel</TabsPanel>
      </Tabs>,
    )

    const ids = Array.from(container.querySelectorAll('[data-oracle-id]')).map((el) =>
      el.getAttribute('data-oracle-id'),
    )

    expect(new Set(ids)).toEqual(
      new Set([
        'tabs',
        'tabs-list',
        'tabs-tab-workspaces',
        'tabs-tab-chats',
        'tabs-tab-files',
        'tab-indicator',
        'tabs-content-workspaces',
      ]),
    )
    // No id repeats. A repeated one is a refusal in the extractor rather than a
    // delta, so this is the property the derived suffix exists for.
    expect(ids).toHaveLength(new Set(ids).size)
  })

  it('puts each id on the element that carries the matching data-slot', () => {
    const { container } = render(
      <Tabs defaultValue="changes">
        <TabsList variant="default">
          <TabsTab value="changes">Changes</TabsTab>
          <TabsTab value="history">History</TabsTab>
        </TabsList>
        <TabsPanel value="changes">panel</TabsPanel>
      </Tabs>,
    )

    for (const [slot, id] of [
      ['tabs', 'tabs'],
      ['tabs-list', 'tabs-list'],
      ['tab-indicator', 'tab-indicator'],
      ['tabs-tab', 'tabs-tab-changes'],
      ['tabs-content', 'tabs-content-changes'],
    ]) {
      const el = container.querySelector(`[data-oracle-id="${id}"]`)
      expect(el?.getAttribute('data-slot')).toBe(slot)
    }

    // Only the active panel is mounted — base-ui drops the other unless a call
    // site asks for `keepMounted`, and none does.
    expect(container.querySelector('[data-oracle-id="tabs-content-history"]')).toBeNull()
  })

  it('lets a call site override the id, because it is written before the spread', () => {
    const { container } = render(
      <Tabs defaultValue="a">
        <TabsList variant="default">
          <TabsTab value="a" data-oracle-id="tabs-tab-renamed">
            A
          </TabsTab>
        </TabsList>
      </Tabs>,
    )

    expect(container.querySelector('[data-oracle-id="tabs-tab-renamed"]')).not.toBeNull()
    expect(container.querySelector('[data-oracle-id="tabs-tab-a"]')).toBeNull()
  })

  it('tags nothing outside the primitive, so a tab’s children stay invisible', () => {
    const { container } = render(
      <Tabs defaultValue="workspaces">
        <TabsList variant="default">
          <TabsTab value="workspaces">
            <svg />
            <span>Workspaces</span>
          </TabsTab>
        </TabsList>
      </Tabs>,
    )

    const tab = container.querySelector('[data-oracle-id="tabs-tab-workspaces"]')
    expect(tab?.querySelectorAll('[data-oracle-id]')).toHaveLength(0)
    // And the tab has no text node of its own, which is why no anchor on this
    // surface reports the contract's text group.
    expect(
      Array.from(tab?.childNodes ?? []).filter((node) => node.nodeType === Node.TEXT_NODE),
    ).toHaveLength(0)
  })
})
