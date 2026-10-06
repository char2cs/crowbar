import { createChat, createWorkspaceChat, getChat } from '../lib/fixture.mjs'
import { liveStubProcesses } from '../lib/stub-provider.mjs'
import { openChat } from '../lib/ui.mjs'
import { assert, assertEqual, until } from '../lib/wait.mjs'

// Closing a view ends the chat's provider process (the chat stays, dormant and
// resumable); a chat that is still in another view, or a sibling chat of the
// same workspace, keeps its process.

const closeRow = (id) => {
  document.querySelector(`[data-testid="recents-row-${id}"] button[aria-label^="Close "]`).click()
  return true
}

export default {
  name: 'close-view',
  async run({ d, daemon, fx, inst }) {
    const a = await createWorkspaceChat(daemon, fx)
    const b = await createWorkspaceChat(daemon, fx)
    const sibling = await createChat(daemon, fx, a.workspaceId)
    const baseline = liveStubProcesses(inst.runDir)
    assert(baseline >= 3, `three stub processes are up before the close (saw ${baseline})`)

    for (const chat of [a, b, sibling]) await openChat(d, chat.id)

    await d.eval(closeRow, a.id)
    await until('the closed chat to go dormant', async () => {
      const chat = await getChat(daemon, fx, a.id)
      return chat.phase === 'dormant' && !chat.liveRunnerId
    })
    await until(
      'its provider process to exit',
      () => liveStubProcesses(inst.runDir) === baseline - 1,
    )

    for (const kept of [b, sibling]) {
      const chat = await getChat(daemon, fx, kept.id)
      assert(
        chat.liveRunnerId,
        `${kept.id} must keep its runner when only another chat's view closed`,
      )
    }
    assertEqual(liveStubProcesses(inst.runDir), baseline - 1, 'exactly one provider process ended')

    // Closing ended the view, never the chat: it is still there, dormant.
    assertEqual((await getChat(daemon, fx, a.id)).id, a.id, 'the closed chat still exists')
  },
}
