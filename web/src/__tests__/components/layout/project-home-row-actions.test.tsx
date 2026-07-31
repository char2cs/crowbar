/**
 * The Project Home row's trailing action is a 24x24 icon-only control with no
 * visible label, so a tooltip is the ONLY way a sighted mouse user learns what
 * it does. It was once rewritten as a raw <button> carrying the bare
 * ROW_SUB_ACTION class string, which dropped the tooltip and side-stepped the
 * `@/components/ui/*` primitive rule. `aria-label` survived, so that was purely
 * a sighted-discoverability regression.
 *
 * jsdom has no layout engine, so density is asserted on the class contract (the
 * shared ROW_SUB_ACTION chrome plus a fixed size-6 = 24px box) rather than on a
 * measured rect.
 *
 * "Switch project" used to sit beside it. It is gone: the sidebar now holds a
 * row per project, so the panel it pushed had nothing left to show.
 */
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// The row's glyphs are Lucide, which shares the process's React copy and so needs
// no stub here (@phosphor-icons/react does — its pure-ESM build picks up its own
// React and throws "Cannot read properties of null (reading 'useContext')").
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useMatch: () => null,
}))

// The row joins the tree's drag system (it is a drop target for repos and a
// drag subject itself), so it reads both halves of the tree context. Driven
// directly here: these tests render the row on its own, with no provider.
const drag = vi.hoisted(() => ({
  draggingWs: null,
  draggingIds: new Set<string>(),
  dropTarget: null as { kind: string; id: string; mode: string } | null,
  movingIds: new Set<string>(),
}))
const actions = vi.hoisted(() => ({ onPointerDownDrag: vi.fn() }))

vi.mock('@/components/layout/workspace-tree-context', () => ({
  useWorkspaceTreeActions: () => actions,
  useWorkspaceTreeDrag: () => drag,
}))

import { ProjectHomeRow } from '@/components/layout/project-home-row'
import { ROW_SUB_ACTION } from '@/components/layout/workspace-row-base'

const LUCIDE_DEFAULT_STROKE = 2
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useProjectStore } from '@/lib/store/projects'

beforeEach(() => {
  useProjectStore.setState({ activeProjectId: 'p1', projects: [] })
  useHomeWorkspaceStore.setState({ workspace: null })
})

const renderRow = () => render(<ProjectHomeRow project={{ id: 'p1', name: 'home' }} isCollapsed />)

const ACTIONS = ['Import repository'] as const

describe('ProjectHomeRow trailing actions', () => {
  it.each(ACTIONS)('names %s in a tooltip on hover', async (label) => {
    const user = userEvent.setup()
    renderRow()

    await user.hover(screen.getByRole('button', { name: label }))

    // Radix renders the tooltip text twice in jsdom (visible + a11y node).
    expect((await screen.findAllByText(label))[0]).toBeInTheDocument()
  })

  it.each(ACTIONS)('routes %s through the shared Button primitive', (label) => {
    renderRow()

    expect(screen.getByRole('button', { name: label })).toHaveAttribute('data-slot', 'button')
  })

  it.each(ACTIONS)('keeps %s on the shared row sub-action chrome', (label) => {
    renderRow()

    const button = screen.getByRole('button', { name: label })
    for (const chromeClass of ROW_SUB_ACTION.trim().split(/\s+/)) {
      expect(button.className.split(/\s+/), `${label} lost ${chromeClass}`).toContain(chromeClass)
    }
  })

  it.each(ACTIONS)('keeps %s a 24px box around a 12px glyph', (label) => {
    renderRow()

    const button = screen.getByRole('button', { name: label })
    const classes = button.className.split(/\s+/)
    // size-6 == 1.5rem == 24px, unconditional (no sm: breakpoint step).
    expect(classes, `${label} is not pinned to 24px`).toContain('size-6')
    expect(classes.filter((c) => /^(sm:)?(size|h|w)-(7|8|9|10|11)$/.test(c))).toEqual([])
    // The glyph must carry a size-* class of its OWN: buttonVariants' base rule
    // only sizes svgs matching :not([class*='size-']), so this is what keeps the
    // 12px glyph from being blown up to the variant's 14px.
    expect(button.querySelector('svg')?.getAttribute('class')?.split(/\s+/)).toContain('size-3')
  })

  // These glyphs were once given an explicit strokeWidth to match the hand-rolled
  // 16-unit `+` in repo-section.tsx (2/16 = 1.5px at this box). That made them the
  // boldest marks in the sidebar: every OTHER Lucide icon here — the header
  // cluster, the sidebar toggle, the repo row's own Import branches — runs at
  // Lucide's default 2, i.e. 1.0px in a 12px box. The default is the house weight,
  // so the correct value for these is no override at all.
  it.each(ACTIONS)('leaves %s at Lucide default weight', (label) => {
    renderRow()

    const svg = screen.getByRole('button', { name: label }).querySelector('svg')
    const stroke = Number(svg?.getAttribute('stroke-width'))

    expect(stroke).toBe(LUCIDE_DEFAULT_STROKE)
    // Lucide draws on a 24-unit viewBox; size-3 renders it into a 12px box.
    expect((stroke / 24) * 12).toBeCloseTo(1, 5)
  })
})
