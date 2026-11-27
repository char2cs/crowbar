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
