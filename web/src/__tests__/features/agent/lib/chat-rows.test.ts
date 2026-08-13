/**
 * `buildChatTree` and `chatSubtreeIds` — the flat row list the Chats panel
 * renders, and the subtree walk a delete uses.
 *
 * The invariants under test are the ones the source's own doc comments
 * promise: a dangling parent lands at the root rather than vanishing, a parent
 * cycle terminates and still renders every row, search keeps a match's whole
 * subtree and dims its ancestors, and a folded row that is holding an
 * on-screen chat hoists that one chat under it. Nothing here re-derives the
 * implementation — these are the contracts a future rewrite still has to keep.
 */
import { describe, expect, it } from 'vitest'
import {
  buildChatTree,
  chatSubtreeIds,
  type ChatLike,
  type ChatRow,
  type ChatTreeInput,
  type FolderLike,
} from '@/features/agent/lib/chat-rows'

// --- fixtures ---------------------------------------------------------------

const chat = (id: string, over: Partial<ChatLike> = {}): ChatLike => ({
  id,
  title: `Chat ${id}`,
  activeProviderId: 'claude',
  createdAt: '2024-01-01T00:00:00.000Z',
  ...over,
})

const folder = (id: string, over: Partial<FolderLike> = {}): FolderLike => ({
  id,
  name: `Folder ${id}`,
  ...over,
})

/** An input with every collection empty, so a test only has to override what it needs. */
const input = (over: Partial<ChatTreeInput> = {}): ChatTreeInput => ({
  chats: [],
  folders: [],
  collapsed: new Set(),
  shown: new Set(),
  foldedAway: new Set(),
  query: '',
  ...over,
})

const ids = (rows: ChatRow[]) => rows.map((r) => r.id)
const byId = (rows: ChatRow[], id: string): ChatRow => {
  const row = rows.find((r) => r.id === id)
  if (!row) throw new Error(`no row for ${id}`)
  return row
}

describe('sibling order', () => {
  it('sorts ascending by order, whatever order the input arrived in', () => {
    const { rows } = buildChatTree(
      input({
        chats: [chat('a', { order: 2 }), chat('b', { order: 0 }), chat('c', { order: 1 })],
      }),
    )
    expect(ids(rows)).toEqual(['b', 'c', 'a'])
  })

  it('breaks a tie between a folder and a chat with the folder above', () => {
    // Both default to order 0. A folder just created must not appear below a
    // screenful of chats that happen to share its order.
    const { rows } = buildChatTree(input({ chats: [chat('c1')], folders: [folder('f1')] }))
    expect(ids(rows)).toEqual(['f1', 'c1'])
  })

  it('breaks a tie among chats by newest-createdAt-first', () => {
    const { rows } = buildChatTree(
      input({
        chats: [
          chat('old', { createdAt: '2024-01-01T00:00:00.000Z' }),
          chat('new', { createdAt: '2024-06-01T00:00:00.000Z' }),
          chat('mid', { createdAt: '2024-03-01T00:00:00.000Z' }),
        ],
      }),
    )
    expect(ids(rows)).toEqual(['new', 'mid', 'old'])
  })

  it('breaks a full tie by id, so identical data can never reshuffle on a re-render', () => {
    const same = { order: 0, createdAt: '2024-01-01T00:00:00.000Z' }
    const { rows } = buildChatTree(
      input({ chats: [chat('z', same), chat('a', same), chat('m', same)] }),
    )
    expect(ids(rows)).toEqual(['a', 'm', 'z'])

    // Same data, reversed insertion order: the id tie-break is symmetric, not
    // an artifact of whichever pair the sort happened to compare first.
    const { rows: reversed } = buildChatTree(
      input({ chats: [chat('m', same), chat('z', same), chat('a', same)] }),
    )
    expect(ids(reversed)).toEqual(['a', 'm', 'z'])
  })

  it('defaults a missing order and parentId to 0 and root', () => {
    const { rows } = buildChatTree(input({ chats: [{ ...chat('c1'), order: undefined }] }))
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({ id: 'c1', parentId: '', depth: 0 })
  })
})

describe('placement — chats, folders and threads nest freely', () => {
  it('a chat may hold another chat as a thread, at depth + 1', () => {
    const { rows } = buildChatTree(
      input({ chats: [chat('parent'), chat('thread', { parentId: 'parent' })] }),
    )
    expect(byId(rows, 'thread')).toMatchObject({ parentId: 'parent', depth: 1, path: '/parent/' })
  })

  it('a folder may hold anything, including a row inside a chat', () => {
    const { rows } = buildChatTree(
      input({
        chats: [chat('parent'), chat('thread', { parentId: 'parent' })],
        folders: [folder('f1', { parentId: 'thread' })],
      }),
    )
    expect(byId(rows, 'f1')).toMatchObject({
      parentId: 'thread',
      depth: 2,
      path: '/parent/thread/',
    })
  })

  it('an unknown parentId renders the row at the root rather than dropping it', () => {
    // The daemon's delete-the-parent-first race: the row still exists in the
    // store, and the worst outcome here is a chat nobody can find or move.
    const { rows } = buildChatTree(input({ chats: [chat('orphan', { parentId: 'ghost' })] }))
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({ id: 'orphan', parentId: '', depth: 0, path: '/' })
  })

  it('hasChildren is true only for a row with at least one real child', () => {
    const { rows } = buildChatTree(
      input({ chats: [chat('parent'), chat('leaf'), chat('thread', { parentId: 'parent' })] }),
    )
    expect(byId(rows, 'parent').hasChildren).toBe(true)
    expect(byId(rows, 'leaf').hasChildren).toBe(false)
    expect(byId(rows, 'thread').hasChildren).toBe(false)
  })
})

describe('a parent cycle terminates and still renders every row, at the root', () => {
  it('a mutual cycle between two folders renders both, rather than hanging', () => {
    // f1 and f2 each name the other as parent. Neither is ever reachable from
    // the real root, so both are picked up by the leftover pass afterwards.
    const { rows } = buildChatTree(
      input({ folders: [folder('f1', { parentId: 'f2' }), folder('f2', { parentId: 'f1' })] }),
    )
    expect(ids(rows).sort()).toEqual(['f1', 'f2'])
    for (const row of rows) expect(row).toMatchObject({ depth: 0, path: '/' })
  })

  it('a chat that names itself as its own parent still renders once', () => {
    const { rows } = buildChatTree(input({ chats: [chat('self', { parentId: 'self' })] }))
    expect(ids(rows)).toEqual(['self'])
    expect(rows[0]).toMatchObject({ depth: 0, path: '/' })
  })

  it('publishes the real (cyclic) parentId on the row even though it renders at the root', () => {
    // depth/path say "root" for safety, but the row still BELONGS to its real
    // container — the same rule kept rows follow, applied to a row a cycle
    // pushed out of the walk rather than one a fold hid.
    const { rows } = buildChatTree(
      input({ folders: [folder('f1', { parentId: 'f2' }), folder('f2', { parentId: 'f1' })] }),
    )
    expect(byId(rows, 'f1').parentId).toBe('f2')
    expect(byId(rows, 'f2').parentId).toBe('f1')
  })

  it('does not hang when a shown chat is folded behind a cyclic ancestor chain', () => {
    // Walking UP from a shown chat to find its holder has its own cycle guard
    // (`walked`), separate from the row walk's. This exercises both at once:
    // x -> f1 -> f2 -> f1 (cycle), with x itself shown and both folders
    // collapsed. The whole cluster is unreachable from root, so every row
    // still renders — this test times out if either guard is missing.
    const { rows } = buildChatTree(
      input({
        chats: [chat('x', { parentId: 'f1' }), chat('y', { parentId: 'f1' })],
        folders: [folder('f1', { parentId: 'f2' }), folder('f2', { parentId: 'f1' })],
        collapsed: new Set(['f1', 'f2']),
        shown: new Set(['x', 'y']),
      }),
    )
    expect(ids(rows).sort()).toEqual(['f1', 'f2', 'x', 'y'])
    for (const row of rows) expect(row.depth).toBe(0)
  })
})

describe('search', () => {
  it('keeps a match and its whole subtree', () => {
    const { rows } = buildChatTree(
      input({
        chats: [
          chat('root', { title: 'Widgets' }),
          chat('kid', { title: 'unrelated', parentId: 'root' }),
          chat('grandkid', { title: 'also unrelated', parentId: 'kid' }),
          chat('other', { title: 'nothing to do with it' }),
        ],
        query: 'widget',
      }),
    )
    expect(ids(rows).sort()).toEqual(['grandkid', 'kid', 'root'])
  })

  it('keeps an ancestor of a match as dimmed context, not as a hit', () => {
    const { rows } = buildChatTree(
      input({
        chats: [
          chat('parent', { title: 'Spikes' }),
          chat('kid', { title: 'widget', parentId: 'parent' }),
        ],
        query: 'widget',
      }),
    )
    expect(byId(rows, 'parent').ctx).toBe(true)
    expect(byId(rows, 'kid').ctx).toBe(false)
  })

  it('ignores collapse while a query is active — a folded parent cannot hide the only result', () => {
    const { rows } = buildChatTree(
      input({
        chats: [
          chat('parent', { title: 'Spikes' }),
          chat('kid', { title: 'widget', parentId: 'parent' }),
        ],
        collapsed: new Set(['parent']),
        query: 'widget',
      }),
    )
    expect(byId(rows, 'kid').kept).toBe(false)
    expect(byId(rows, 'parent').expanded).toBe(true)
    expect(byId(rows, 'kid').expanded).toBe(true)
  })

  it('produces no rows at all for a query nothing matches', () => {
    const { rows } = buildChatTree(
      input({ chats: [chat('c1', { title: 'Widgets' })], query: 'nope' }),
    )
    expect(rows).toEqual([])
  })

  it('a blank/whitespace query is not filtering at all', () => {
    const { rows } = buildChatTree(input({ chats: [chat('c1')], query: '   ' }))
    expect(rows).toHaveLength(1)
    expect(rows[0].ctx).toBe(false)
  })

  it('still renders every row of a search that reaches a cyclic, unreachable cluster', () => {
    // The cluster is never in the search forest (it is not reachable from
    // root either), so it cannot match — but the leftover pass must still be
    // able to run its own `sets`-aware skip without hanging or throwing.
    const { rows } = buildChatTree(
      input({
        folders: [folder('f1', { parentId: 'f2' }), folder('f2', { parentId: 'f1' })],
        chats: [chat('findme', { title: 'widget' })],
        query: 'widget',
      }),
    )
    expect(ids(rows)).toEqual(['findme'])
  })
})

describe('kept rows — a folded row holding an on-screen chat', () => {
  it('hoists a shown chat one level under its folded holder and marks the holder as holding', () => {
    const { rows } = buildChatTree(
      input({
        chats: [chat('parent'), chat('deep', { parentId: 'parent' })],
        collapsed: new Set(['parent']),
        shown: new Set(['deep']),
      }),
    )
    expect(byId(rows, 'parent')).toMatchObject({ holding: true, expanded: false })
    expect(byId(rows, 'deep')).toMatchObject({ kept: true, depth: 1, parentId: 'parent' })
  })

  it('the outermost collapsed ancestor is the holder, not the nearest one', () => {
    // 'inner' is itself folded away with 'outer' — only a CHAT can ever be
    // kept, so the intermediate folder does not render at all, and 'leaf'
    // hoists all the way up to the outer row rather than stopping at 'inner'.
    const { rows } = buildChatTree(
      input({
        folders: [folder('outer'), folder('inner', { parentId: 'outer' })],
        chats: [chat('leaf', { parentId: 'inner' })],
        collapsed: new Set(['outer', 'inner']),
        shown: new Set(['leaf']),
      }),
    )
    expect(ids(rows).sort()).toEqual(['leaf', 'outer'])
    expect(byId(rows, 'outer').holding).toBe(true)
    const kept = byId(rows, 'leaf')
    // Only its DEPTH is the holder's — outer's + 1, not inner's. Its parent and
    // its chain are its OWN, all the way down to 'inner', because both are what
    // a drag reads off the row: `parentId` is the level a drop plans into, and
    // `path` is what the own-descendant refusal is a substring test on. Handing
    // either of them the holder's answer is how a subtree gets detached.
    expect(kept).toMatchObject({
      kept: true,
      depth: 1,
      path: '/outer/inner/',
      parentId: 'inner',
    })
  })

  it('a kept row publishes the chain it LIVES in, however far it was hoisted', () => {
    // Four levels deep, drawn one step under the row that swallowed it. The
    // chain is walked up from the row rather than accumulated on the way down,
    // because the walk down never reaches it.
    const { rows } = buildChatTree(
      input({
        folders: [folder('a'), folder('b', { parentId: 'a' }), folder('c', { parentId: 'b' })],
        chats: [chat('leaf', { parentId: 'c' })],
        collapsed: new Set(['a']),
        shown: new Set(['leaf']),
      }),
    )
    expect(byId(rows, 'leaf')).toMatchObject({ kept: true, depth: 1, path: '/a/b/c/' })
  })

  it('a kept row at the tree ROOT publishes the root chain', () => {
    // Reachable when the holder is a chat: the kept row is its thread, so the
    // chain has exactly one link.
    const { rows } = buildChatTree(
      input({
        chats: [chat('parent'), chat('thread', { parentId: 'parent' })],
        collapsed: new Set(['parent']),
        shown: new Set(['thread']),
      }),
    )
    expect(byId(rows, 'thread').path).toBe('/parent/')
  })

  it('a kept row advertises no children — its own subtree is not hoisted with it', () => {
    // `hasChildren` is what draws the chevron AND what makes the gap under a row
    // mean "before its first child" to a drop. A kept row is hoisted alone, so
    // both would be about rows that are not on screen.
    const { rows } = buildChatTree(
      input({
        chats: [
          chat('folded'),
          chat('kid', { parentId: 'folded' }),
          chat('grandkid', { parentId: 'kid' }),
        ],
        collapsed: new Set(['folded']),
        shown: new Set(['kid']),
      }),
    )
    expect(ids(rows).sort()).toEqual(['folded', 'kid'])
    expect(byId(rows, 'kid')).toMatchObject({ kept: true, hasChildren: false })
    // …and the row drawn in its own place still says what it really holds.
    expect(byId(rows, 'folded').hasChildren).toBe(true)
  })

  it('walks past an ordinary (uncollapsed) ancestor to find a further, folded one', () => {
    // 'middle' sits between 'leaf' and 'outer' but is not itself collapsed —
    // the holder search must not stop or mis-set on it, only on a row that is
    // actually folded.
    const { rows } = buildChatTree(
      input({
        folders: [folder('outer'), folder('middle', { parentId: 'outer' })],
        chats: [chat('leaf', { parentId: 'middle' })],
        collapsed: new Set(['outer']),
        shown: new Set(['leaf']),
      }),
    )
    expect(ids(rows).sort()).toEqual(['leaf', 'outer'])
    expect(byId(rows, 'outer').holding).toBe(true)
    expect(byId(rows, 'leaf')).toMatchObject({ kept: true, depth: 1 })
  })

  it('foldedAway suppresses the kept row and the holder goes back to not-holding', () => {
    const { rows } = buildChatTree(
      input({
        chats: [chat('parent'), chat('deep', { parentId: 'parent' })],
        collapsed: new Set(['parent']),
        shown: new Set(['deep']),
        foldedAway: new Set(['parent']),
      }),
    )
    expect(ids(rows)).toEqual(['parent'])
    expect(byId(rows, 'parent').holding).toBe(false)
  })

  it('orders two kept rows in tree order, not Set-insertion order', () => {
    const { rows } = buildChatTree(
      input({
        chats: [
          chat('parent'),
          chat('first', { parentId: 'parent', order: 0 }),
          chat('second', { parentId: 'parent', order: 1 }),
        ],
        collapsed: new Set(['parent']),
        // Inserted backwards — the output must still read top to bottom.
        shown: new Set(['second', 'first']),
      }),
    )
    expect(ids(rows)).toEqual(['parent', 'first', 'second'])
  })

  it('a shown chat with no folded ancestor is not held by anything', () => {
    const { rows } = buildChatTree(input({ chats: [chat('c1')], shown: new Set(['c1']) }))
    expect(byId(rows, 'c1')).toMatchObject({ kept: false, holding: false })
  })

  it('ignores a shown id that names no chat in the tree', () => {
    const { rows } = buildChatTree(input({ chats: [chat('c1')], shown: new Set(['ghost']) }))
    expect(rows).toHaveLength(1)
  })

  it('a chat under a collapsed parent that is not shown simply does not appear', () => {
    const { rows } = buildChatTree(
      input({
        chats: [chat('parent'), chat('hidden', { parentId: 'parent' })],
        collapsed: new Set(['parent']),
      }),
    )
    expect(ids(rows)).toEqual(['parent'])
  })
})

describe('the siblings index', () => {
  it('indexes the real children of a level, independent of what is drawn', () => {
    const { siblings } = buildChatTree(
      input({
        chats: [chat('a', { order: 1 }), chat('b', { order: 0 })],
        folders: [folder('f1', { order: 2 })],
      }),
    )
    expect(siblings.get('')).toEqual(['b', 'a', 'f1'])
  })

  it('still lists a folded row’s real children, even though they are not drawn', () => {
    const { siblings } = buildChatTree(
      input({
        chats: [chat('parent'), chat('kid', { parentId: 'parent' })],
        collapsed: new Set(['parent']),
      }),
    )
    expect(siblings.get('parent')).toEqual(['kid'])
  })
})

describe('chatSubtreeIds', () => {
  it('lists every descendant, deepest first, excluding the node itself', () => {
    const siblings = new Map<string, string[]>([
      ['root', ['c1', 'c2']],
      ['c1', ['g1']],
    ])
    expect(chatSubtreeIds(siblings, 'root')).toEqual(['g1', 'c1', 'c2'])
  })

  it('returns nothing for a node with no siblings entry at all', () => {
    expect(chatSubtreeIds(new Map(), 'leaf')).toEqual([])
  })

  it('terminates and excludes itself even if the siblings map itself cycles', () => {
    const siblings = new Map<string, string[]>([
      ['a', ['b']],
      ['b', ['a']],
    ])
    expect(chatSubtreeIds(siblings, 'a')).toEqual(['b'])
  })

  it('walks the real siblings index a build produced, for a folder full of threads', () => {
    const { siblings } = buildChatTree(
      input({
        folders: [folder('f1')],
        chats: [
          chat('c1', { parentId: 'f1' }),
          chat('c2', { parentId: 'f1' }),
          chat('thread', { parentId: 'c1' }),
        ],
      }),
    )
    // Deepest first: the thread before its own chat, both before the sibling.
    expect(chatSubtreeIds(siblings, 'f1')).toEqual(['thread', 'c1', 'c2'])
  })
})

// --- G6: the build is linear, not quadratic --------------------------------

/**
 * A realistic mix: a batch of root items, most of them chats with a couple of
 * folders sprinkled in, and threads nested two or three deep off a rotating
 * pool of recent parents. Every id is unique across the whole call so a build
 * of `n` never collides with a build of `2n`.
 */
function makeRealisticInput(n: number, tag: string): { chats: ChatLike[]; folders: FolderLike[] } {
  const chats: ChatLike[] = []
  const folders: FolderLike[] = []
  const rootBudget = Math.max(1, Math.round(n * 0.25))
  const parents: { id: string; depth: number }[] = []
  let order = 0

  for (let i = 0; i < n; i++) {
    const isFolder = i % 11 === 0
    const isRoot = i < rootBudget || parents.length === 0
    const parent = isRoot ? undefined : parents[i % parents.length]
    const parentId = parent?.id
    const depth = parent ? parent.depth + 1 : 0
    const id = `${tag}-${isFolder ? 'f' : 'c'}${i}`

    if (isFolder) {
      folders.push({ id, name: `Folder ${i}`, parentId, order: order++ })
    } else {
      chats.push({
        id,
        title: `Chat ${i}`,
        activeProviderId: 'claude',
        createdAt: new Date(i).toISOString(),
        parentId,
        order: order++,
      })
    }
    // Cap nesting at 3 deep, the "two or three deep" thread shape.
    if (depth < 3) parents.push({ id, depth })
  }
  return { chats, folders }
}

/**
 * The best (lowest) of several timed builds, one size at a time — but
 * INTERLEAVED across sizes rather than one size's whole batch then the next.
 * A GC pause or a neighbouring test's cleanup lands on whichever round is
 * running when it happens; interleaving spreads that noise evenly over every
 * size instead of letting it hit only the batch that happened to run during
 * it, which is what made a size-at-a-time sweep occasionally read the small
 * size unrealistically fast (or the large one unrealistically slow) under
 * coverage instrumentation's extra GC pressure.
 */
function bestBuildTimes(
  sized: readonly { label: string; input: ChatTreeInput }[],
  runs: number,
): Record<string, number> {
  const best: Record<string, number> = {}
  for (const { label } of sized) best[label] = Infinity
  for (let i = 0; i < runs; i++) {
    for (const { label, input } of sized) {
      const start = performance.now()
      buildChatTree(input)
      const elapsed = performance.now() - start
      if (elapsed < best[label]) best[label] = elapsed
    }
  }
  return best
}

describe('G6 — the row build scales linearly with the number of rows', () => {
  it('costs roughly the same per row from 1k to 4k, not quadratically more', () => {
    const asInput = (size: { chats: ChatLike[]; folders: FolderLike[] }): ChatTreeInput => ({
      ...size,
      collapsed: new Set(),
      shown: new Set(),
      foldedAway: new Set(),
      query: '',
    })

    const size1k = makeRealisticInput(1000, 'a')
    const size2k = makeRealisticInput(2000, 'b')
    const size4k = makeRealisticInput(4000, 'c')
    const input1k = asInput(size1k)
    const input2k = asInput(size2k)
    const input4k = asInput(size4k)
    const sized = [
      { label: '1k', input: input1k },
      { label: '2k', input: input2k },
      { label: '4k', input: input4k },
    ]

    // Warm up the JIT before measuring — an interpreted first pass would
    // otherwise dominate the smallest size and understate its real cost.
    for (let i = 0; i < 5; i++) {
      for (const { input } of sized) buildChatTree(input)
    }

    const { '1k': t1k, '2k': t2k, '4k': t4k } = bestBuildTimes(sized, 20)

    // A quadratic build is ~4x from 1k->2k and ~16x from 1k->4k; linear plus
    // the sort's log factor stays well under that. Floor t1k so a sub-ms
    // sample (common on a fast machine) cannot make the ratio explode on
    // timer-resolution noise alone.
    const ratio = t4k / Math.max(t1k, 0.05)
    expect(ratio).toBeLessThan(8)
    // A quadratic 1k->2k step alone would already clear 3x; catch it even if
    // 4k somehow lands within budget.
    expect(t2k / Math.max(t1k, 0.05)).toBeLessThan(4)

    // Linear also means "no per-row tree walk crept in": every id the input
    // named comes back exactly once, nothing dropped and nothing duplicated.
    const { rows } = buildChatTree(input4k)
    const inputIds = [...size4k.chats.map((c) => c.id), ...size4k.folders.map((f) => f.id)]
    expect(rows).toHaveLength(inputIds.length)
    const rowIds = new Set(rows.map((r) => r.id))
    expect(rowIds.size).toBe(inputIds.length)
    for (const id of inputIds) expect(rowIds.has(id)).toBe(true)
  })

  it('with 1000 chats, rows.length is exactly the visible count — a folded child does not appear', () => {
    // 998 chats-only filler (no folders, so the count is exact), plus one
    // known root/child pair appended on top — so which chat disappears when
    // its parent folds is asserted by id, not left to the generator's own
    // (unrelated) branching to happen to produce one.
    const filler: ChatLike[] = []
    for (let i = 0; i < 998; i++) {
      filler.push(
        chat(`p-c${i}`, {
          createdAt: new Date(i).toISOString(),
          parentId: i > 0 && i % 3 === 0 ? `p-c${i - 1}` : undefined,
        }),
      )
    }
    const rootId = 'p-root'
    const childId = 'p-child'
    const chats: ChatLike[] = [
      ...filler,
      chat(rootId, { createdAt: new Date(2000).toISOString() }),
      chat(childId, { parentId: rootId, createdAt: new Date(2001).toISOString() }),
    ]
    expect(chats).toHaveLength(1000)

    const { rows } = buildChatTree(input({ chats, collapsed: new Set([rootId]) }))

    // Everything renders except the one child folded away with its parent.
    expect(rows).toHaveLength(999)
    expect(rows.some((r) => r.id === childId)).toBe(false)
    expect(rows.some((r) => r.id === rootId)).toBe(true)
  })
})
