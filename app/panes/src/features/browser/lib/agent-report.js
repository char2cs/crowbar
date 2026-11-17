/* global window -- runs in the page, not the app */
// Injected as `(<this file>)(id, work)`: runs `work` for the agent request `id` and posts its
// return value, or its failure, to the shell, the only channel a page has.
// eslint-disable-next-line @typescript-eslint/no-unused-vars -- the host evaluates this file as text, so nothing here references the name
async function agentReport(id, work) {
  const post = (body) =>
    window.ipc.postMessage(JSON.stringify({ crowbarAgent: Object.assign({ id }, body) }))
  try {
    const value = await work()
    post({ result: value === undefined ? null : value })
  } catch (error) {
    post({ error: String((error && error.message) || error) })
  }
}
