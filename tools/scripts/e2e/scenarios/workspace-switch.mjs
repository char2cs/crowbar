import { createWorkspaceChat } from '../lib/fixture.mjs'
import { focusRecent, openChat } from '../lib/ui.mjs'
import { assert, assertEqual } from '../lib/wait.mjs'

// Every chat is its own workspace, so moving focus between chats of different
// workspaces is a workspace switch. A warm workspace keeps its file tree and
// git state live, so switching to it must fetch nothing.

const WORKSPACES = 4
const WORKSPACE_STATE = /\/(files|git|review)(\/|\?|$)|\/threads/

const recordFetches = () => {
  if (!window.__e2eFetch) {
    window.__e2eFetch = []
    const original = window.fetch
    window.fetch = function (input, init) {
      window.__e2eFetch.push(String(typeof input === 'string' ? input : (input.url ?? input)))
      return original.call(this, input, init)
    }
  }
  window.__e2eFetch.length = 0
  return true
}
const fetched = () => [...window.__e2eFetch]
const activeWorkspace = () =>
  document.querySelector('[data-workspace-slot][data-active="true"]')?.dataset.workspaceSlot ?? null

export default {
  name: 'workspace-switch',
  async run({ d, daemon, fx }) {
    const chats = []
    for (let i = 0; i < WORKSPACES; i++) chats.push(await createWorkspaceChat(daemon, fx))

    await d.eval(recordFetches)
    for (const chat of chats) {
      await openChat(d, chat.id)
      await d.until(
        'the opened workspace to be active',
        (ws) =>
          document.querySelector('[data-workspace-slot][data-active="true"]')?.dataset
            .workspaceSlot === ws,
        chat.workspaceId,
      )
    }
    // Positive control: loading cold workspaces does fetch workspace state, so a
    // silent counter cannot make the assertion below pass vacuously.
    const warmup = (await d.eval(fetched)).filter((url) => WORKSPACE_STATE.test(url))
    assert(warmup.length > 0, 'opening cold workspaces must be observed fetching files/git state')

    await d.eval(recordFetches)
    for (let round = 0; round < 3; round++) {
      for (const chat of chats) {
        assert(await focusRecent(d, chat.id), 'the Recents row exists')
        await d.until(
          'the workspace to become active',
          (ws) =>
            document.querySelector('[data-workspace-slot][data-active="true"]')?.dataset
              .workspaceSlot === ws,
          chat.workspaceId,
        )
      }
    }
    const refetched = (await d.eval(fetched)).filter((url) => WORKSPACE_STATE.test(url))
    assertEqual(
      refetched,
      [],
      'switching among warm workspaces refetches no files/git/review/threads state',
    )
    assertEqual(await d.eval(activeWorkspace), chats.at(-1).workspaceId, 'the last switch landed')
  },
}
