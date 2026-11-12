// TEMP measurement probe: removed before finishing.
const q: string[] = []
let last = -1
const tick = (t: number) => {
  const el = document.querySelector<HTMLElement>('.agent-chat .scroll')
  if (el && el.scrollTop !== last) {
    last = el.scrollTop
    q.push(`${t.toFixed(1)},${el.scrollTop.toFixed(1)},${el.scrollHeight}`)
  }
  if (q.length >= 20) {
    void fetch('http://10.10.128.27:8899/', { method: 'POST', mode: 'no-cors', body: q.join('\n') })
    q.length = 0
  }
  requestAnimationFrame(tick)
}
requestAnimationFrame(tick)
