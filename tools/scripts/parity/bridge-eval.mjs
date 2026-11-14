// Usage: bun bridge-eval.mjs <port> '<script body that ends with return ...>'
// Talks the tauri-plugin-mcp-bridge protocol (execute_js). Scripts must be synchronous.
const [, , port, script] = process.argv
const ws = new WebSocket(`ws://127.0.0.1:${port}`)
ws.onopen = () => ws.send(JSON.stringify({ id: '1', command: 'execute_js', args: { script } }))
ws.onmessage = (e) => {
  const m = JSON.parse(e.data)
  if (!m.success) { console.error(m.error); process.exit(1) }
  console.log(typeof m.data === 'string' ? m.data : JSON.stringify(m.data, null, 2))
  process.exit(0)
}
setTimeout(() => { console.error('bridge timeout'); process.exit(2) }, 20000)
