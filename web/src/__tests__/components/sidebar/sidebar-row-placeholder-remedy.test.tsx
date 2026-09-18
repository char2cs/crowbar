import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render } from '@testing-library/react'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { SidebarRowActions } from '@/components/sidebar/sidebar-row-actions'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn(), show: vi.fn() },
}))

/**
 * `placeholderReason` composes copy that names two remedies — "detach it to
 * let Crowbar manage this branch" and "Retry to provision it" — and the row
 * rendered neither control, so the user was told to Retry/Detach and given
 * nowhere to do either. The warning glyph carried no reason at all: an
 * `aria-label` reading "Branch needs provisioning" and nothing a sighted user
 * could hover.
 */

const HELD_AT = '/Users/me/repro-data/repo-beta'

const heldRow: SidebarRowType = {
  id: 'chat-held',
  kind: 'branch',
  parentId: 'repo-home',
  order: 0,
  label: 'main',
  ownsWorktree: true,
  workspaceId: 'ws-held',
  working: false,
  hasView: false,
  branchName: 'main',
  locked: true,
  status: 'locked',
  isPlaceholder: true,
  needsProvisioning: false,
  heldByPath: HELD_AT,
  placeholderReason: `\`main\` is checked out at ${HELD_AT} — detach it to hand this branch to Crowbar.`,
}

function controlsOf(row: SidebarRowType): string[] {
  const { container } = render(
    <SidebarRowActions
      row={row}
      isProjectHome={false}
      expanded={false}
      subActionClass=""
      onCreate={vi.fn()}
      onTrash={vi.fn()}
      onToggleFold={vi.fn()}
    />,
  )
  return Array.from(container.querySelectorAll('[data-control]')).map(
    (el) => el.getAttribute('data-control') ?? '',
  )
}

beforeEach(() => {
  useDetachModalStore.setState({ target: null })
})

describe('a branch row whose worktree is held by somebody else', () => {
  it('offers the Detach its own copy names', () => {
    expect(controlsOf(heldRow)).toContain('detach')
  })

  // Retry is coded to refuse a held branch (ErrBranchStillHeld), so offering
  // it here would be a second dead promise, not a fix for the first.
  it('offers no Retry while a holder still has the branch', () => {
    expect(controlsOf(heldRow)).not.toContain('retry-provision')
  })

  it('opens the detach modal on that branch and holder', () => {
    const { container } = render(
      <SidebarRowActions
        row={heldRow}
        isProjectHome={false}
        expanded={false}
        subActionClass=""
        onCreate={vi.fn()}
        onTrash={vi.fn()}
        onToggleFold={vi.fn()}
      />,
    )
    fireEvent.click(container.querySelector('[data-control="detach"]')!)
    expect(useDetachModalStore.getState().target).toEqual({
      wsId: 'ws-held',
      branch: 'main',
      heldByPath: HELD_AT,
    })
  })

  // The failed-provision row is the other half: no holder to detach, so the
  // verb its copy names is Retry.
  it('offers Retry instead once there is no holder left', () => {
    const controls = controlsOf({
      ...heldRow,
      needsProvisioning: true,
      heldByPath: '',
      placeholderReason: "Crowbar couldn't set up `main`. Retry to provision it.",
    })
    expect(controls).toContain('retry-provision')
    expect(controls).not.toContain('detach')
  })

  it('never offers a remedy on a row with a real worktree', () => {
    const controls = controlsOf({
      ...heldRow,
      isPlaceholder: false,
      heldByPath: '',
      placeholderReason: '',
    })
    expect(controls).not.toContain('detach')
    expect(controls).not.toContain('retry-provision')
  })
})

describe('the glyph on a row with no worktree', () => {
  it('says why, not just that', () => {
    const { container } = render(
      <SidebarRow
        row={{ ...heldRow, needsProvisioning: true }}
        depth={0}
        onOpen={vi.fn()}
        onCreate={vi.fn()}
      />,
    )
    const warning = container.querySelector('svg[aria-label="Branch needs provisioning"]')
    expect(warning?.querySelector('title')?.textContent).toContain(HELD_AT)
  })

  // The own-checkout row draws the same Lock a managed protected branch does —
  // the reason is the only thing that tells them apart, so it must be on it.
  it('titles the Lock it draws for the repo’s own checkout', () => {
    const { container } = render(
      <SidebarRow row={heldRow} depth={0} onOpen={vi.fn()} onCreate={vi.fn()} />,
    )
    expect(container.querySelector('svg[aria-label="Branch needs provisioning"]')).toBeNull()
    const titles = Array.from(container.querySelectorAll('svg title')).map((t) => t.textContent)
    expect(titles.some((t) => t?.includes(HELD_AT))).toBe(true)
  })
})
