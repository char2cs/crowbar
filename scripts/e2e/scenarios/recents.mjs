import { createWorkspaceChat } from '../lib/fixture.mjs'
import { openChat } from '../lib/ui.mjs'
import { assert, assertEqual } from '../lib/wait.mjs'

// Recents: with many views open, switching rows only changes which retained
// workspace is shown. Nothing may mount or unmount, and each switch must paint
// quickly. Mounts are counted with a MutationObserver over the structural
// nodes the plan names.

const VIEWS = 9
const ROUNDS = 3
const TIME_BUDGET_MS = 600

// Clicks each Recents row in order, ROUNDS times, timing click -> its workspace
// slot being the visible one, all inside the page so the clock is the page's own.
const startSwitching = ({ chats, rounds }) => {
  const structural = '[data-workspace-slot],[data-view-root],[data-chat-view],[data-pane-container]'
  const run = {
    done: false,
    mounts: 0,
    unmounts: 0,
    times: [],
    failures: [],
    added: [],
    step: 0,
    total: 0,
  }
  const count = (nodes) =>
    [...nodes].reduce(
      (n, node) =>
        n +
        (node.nodeType === 1
          ? (node.matches(structural) ? 1 : 0) + node.querySelectorAll(structural).length
          : 0),
      0,
    )
  const observer = new MutationObserver((records) => {
    for (const r of records) {
      for (const node of r.addedNodes) {
        if (node.nodeType !== 1) continue
        const hits = [node, ...node.querySelectorAll(structural)].filter((el) =>
          el.matches(structural),
        )
        for (const el of hits) {
          run.added.push(
            `step ${run.step}: ${[...el.attributes]
              .map((a) => a.name)
              .filter((n) => n.startsWith('data-'))
              .join(',')}`,
          )
        }
      }
      run.mounts += count(r.addedNodes)
      run.unmounts += count(r.removedNodes)
    }
  })
  observer.observe(document.body, { childList: true, subtree: true })
  const order = []
  for (let r = 0; r < rounds; r++)
    order.push(...(r % 2 ? [...chats].reverse() : chats).filter((c) => c !== order.at(-1)))
  run.total = order.length
  run.rootsAtStart = document.querySelectorAll('[data-view-root]').length
  let i = 0
  const next = () => {
    if (i >= order.length) {
      observer.disconnect()
      run.done = true
      return
    }
    run.step = i
    const { id, workspaceId } = order[i++]
    const row = document.querySelector(`[data-testid="recents-row-${id}"] [role="treeitem"]`)
    const t0 = performance.now()
    row.click()
    const poll = () => {
      const slot = document.querySelector(`[data-workspace-slot="${workspaceId}"]`)
      if (slot && slot.dataset.active === 'true' && slot.style.display !== 'none') {
        run.times.push(Math.round(performance.now() - t0))
        requestAnimationFrame(next)
      } else if (performance.now() - t0 > 5000) {
        run.failures.push(id)
        requestAnimationFrame(next)
      } else requestAnimationFrame(poll)
    }
    requestAnimationFrame(poll)
  }
  window.__e2eRecents = run
  next()
  return true
}

export default {
  name: 'recents',
  async run({ d, daemon, fx }) {
    const chats = []
    for (let i = 0; i < VIEWS; i++) chats.push(await createWorkspaceChat(daemon, fx))
    // Open every view first: the claim is about SWITCHING among warm views.
    for (const chat of chats) await openChat(d, chat.id)
    // Parked views mount one per idle task after they open; the claim is about
    // switching once all of them are up: every view plus the empty stage, which
    // mounts last of all.
    await d.until(
      'every view to be mounted',
      (n) =>
        document.querySelectorAll('[data-view-root]').length >= n + 1 &&
        document.querySelectorAll('[data-workspace-slot]').length >= n,
      VIEWS,
    )

    await d.eval(startSwitching, { chats, rounds: ROUNDS })
    const run = await d.until(
      'the switching run to finish',
      () => (window.__e2eRecents.done ? window.__e2eRecents : null),
      null,
      { timeout: 120_000 },
    )

    assertEqual(run.failures, [], 'every switch must make its workspace visible')
    assertEqual(
      [run.mounts, run.unmounts, run.added, run.rootsAtStart],
      [0, 0, [], VIEWS + 1],
      'switching Recents rows must not mount or unmount anything',
    )
    assertEqual(run.times.length, run.total, 'every switch was timed')
    const sorted = [...run.times].sort((a, b) => a - b)
    const median = sorted[Math.floor(sorted.length / 2)]
    console.log(
      `    recents: ${run.times.length} switches, median ${median}ms, max ${sorted.at(-1)}ms`,
    )
    assert(
      sorted.at(-1) < TIME_BUDGET_MS,
      `slowest switch ${sorted.at(-1)}ms exceeds ${TIME_BUDGET_MS}ms`,
    )
  },
}
