import { describe, expect, it } from 'vitest'
import { shownChatIds } from '@/features/agent/lib/shown-chats'
import type { LayoutNode } from '@/features/panes/types/pane'

const buf = (id: string, chatId: string) => ({ id, type: 'agentChat' as const, chatId })
const leaf = (id: string): LayoutNode => ({ type: 'pane', id })
const split = (first: LayoutNode, second: LayoutNode): LayoutNode => ({
  type: 'split',
  id: 's',
  direction: 'horizontal',
  sizes: [50, 50],
  first,
  second,
})

describe('shownChatIds', () => {
  it('lights only the active tab of a pane', () => {
    expect([
      ...shownChatIds(
        [buf('b1', 'c1'), buf('b2', 'c2')],
        { p1: { bufferIds: ['b1', 'b2'], activeBufferId: 'b1' } },
        leaf('p1'),
      ),
    ]).toEqual(['c1'])
  })

  it('lights the active tab of every pane in the layout', () => {
    expect(
      [
        ...shownChatIds(
          [buf('b1', 'c1'), buf('b2', 'c2')],
          {
            p1: { bufferIds: ['b1'], activeBufferId: 'b1' },
            p2: { bufferIds: ['b2'], activeBufferId: 'b2' },
          },
          split(leaf('p1'), leaf('p2')),
        ),
      ].sort(),
    ).toEqual(['c1', 'c2'])
  })

  it('reaches panes nested several splits deep', () => {
    expect([
      ...shownChatIds(
        [buf('b1', 'c1')],
        { deep: { bufferIds: ['b1'], activeBufferId: 'b1' } },
        split(leaf('other'), split(leaf('x'), leaf('deep'))),
      ),
    ]).toEqual(['c1'])
  })

  it('does NOT light a pane that exists in the store but not in the layout', () => {
    // The regression this module exists for: bottomLayout's panes are in `panes`
    // and nothing renders them, so a chat parked there is on screen nowhere.
    expect([
      ...shownChatIds(
        [buf('b1', 'c1'), buf('b2', 'c2')],
        {
          p1: { bufferIds: ['b1'], activeBufferId: 'b1' },
          bottom: { bufferIds: ['b2'], activeBufferId: 'b2' },
        },
        leaf('p1'),
      ),
    ]).toEqual(['c1'])
  })

  it('does not light a pane with nothing selected', () => {
    expect(
      shownChatIds(
        [buf('b1', 'c1')],
        { p1: { bufferIds: ['b1'], activeBufferId: null } },
        leaf('p1'),
      ).size,
    ).toBe(0)
  })

  it('does not light an activeBufferId the pane no longer lists', () => {
    // A stale pointer left behind by a close is not something on screen.
    expect(
      shownChatIds([buf('b1', 'c1')], { p1: { bufferIds: [], activeBufferId: 'b1' } }, leaf('p1'))
        .size,
    ).toBe(0)
  })

  it('does not light a chat whose buffer no pane lists', () => {
    expect(shownChatIds([buf('b1', 'c1')], {}, leaf('p1')).size).toBe(0)
  })

  it('ignores buffers that are not agent chats', () => {
    expect(
      shownChatIds(
        [{ id: 'b1', type: 'file' }],
        { p1: { bufferIds: ['b1'], activeBufferId: 'b1' } },
        leaf('p1'),
      ).size,
    ).toBe(0)
  })

  it('ignores an agentChat buffer carrying no chatId', () => {
    expect(
      shownChatIds(
        [{ id: 'b1', type: 'agentChat' }],
        { p1: { bufferIds: ['b1'], activeBufferId: 'b1' } },
        leaf('p1'),
      ).size,
    ).toBe(0)
  })

  it('lights a chat open in two panes exactly once', () => {
    const shown = shownChatIds(
      [buf('b1', 'c1'), buf('b2', 'c1')],
      {
        p1: { bufferIds: ['b1'], activeBufferId: 'b1' },
        p2: { bufferIds: ['b2'], activeBufferId: 'b2' },
      },
      split(leaf('p1'), leaf('p2')),
    )
    expect([...shown]).toEqual(['c1'])
  })
})
