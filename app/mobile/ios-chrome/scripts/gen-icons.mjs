// Generates ios/Sources/CrowbarIconData.swift from the Phosphor icons the web ships
// (@phosphor-icons/react defs) so the phone's native UI draws the desktop's own glyphs.
// Usage: node scripts/gen-icons.mjs [--check]   (--check fails if the file is stale)
import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const config = JSON.parse(readFileSync(join(here, 'icons.json'), 'utf8'))
const defs = join(here, '../../../web/node_modules/@phosphor-icons/react/dist/defs')
const out = join(here, '../ios/Sources/CrowbarIconData.swift')

const num = (n) => String(Math.round(n * 1000) / 1000)

// Tokenise SVG path data into absolute M/L/C/Z with arcs and quadratics turned into cubics.
function normalise(d) {
  const tokens = d.match(/[a-zA-Z]|-?\d*\.?\d+(?:e-?\d+)?/g) ?? []
  const res = []
  let i = 0, cmd = '', x = 0, y = 0, sx = 0, sy = 0, lcx = 0, lcy = 0, lastC = false, lqx = 0, lqy = 0, lastQ = false
  const next = () => parseFloat(tokens[i++])
  const flag = () => {
    // arc flags may be glued to the next number ("a1 1 0 011 1"): split a multi-digit token
    const t = tokens[i]
    if (t.length > 1 && /^[01]/.test(t) && !t.includes('.')) {
      tokens[i] = t.slice(1)
      return parseInt(t[0])
    }
    i++
    return parseInt(t)
  }
  const C = (a, b, c, e, f, g) => { res.push(`C${num(a)} ${num(b)} ${num(c)} ${num(e)} ${num(f)} ${num(g)}`); lcx = c; lcy = e; x = f; y = g; lastC = true; lastQ = false }
  const arc = (rx, ry, phi, fa, fs, ex, ey) => {
    if (rx === 0 || ry === 0) { res.push(`L${num(ex)} ${num(ey)}`); x = ex; y = ey; return }
    const p = (phi * Math.PI) / 180, cp = Math.cos(p), sp = Math.sin(p)
    const dx = (x - ex) / 2, dy = (y - ey) / 2
    const x1 = cp * dx + sp * dy, y1 = -sp * dx + cp * dy
    rx = Math.abs(rx); ry = Math.abs(ry)
    const lam = (x1 * x1) / (rx * rx) + (y1 * y1) / (ry * ry)
    if (lam > 1) { rx *= Math.sqrt(lam); ry *= Math.sqrt(lam) }
    const sgn = fa === fs ? -1 : 1
    const num2 = rx * rx * ry * ry - rx * rx * y1 * y1 - ry * ry * x1 * x1
    const co = sgn * Math.sqrt(Math.max(0, num2 / (rx * rx * y1 * y1 + ry * ry * x1 * x1)))
    const cxp = (co * rx * y1) / ry, cyp = (-co * ry * x1) / rx
    const cx = cp * cxp - sp * cyp + (x + ex) / 2, cy = sp * cxp + cp * cyp + (y + ey) / 2
    const ang = (ux, uy, vx, vy) => Math.atan2(ux * vy - uy * vx, ux * vx + uy * vy)
    let th1 = ang(1, 0, (x1 - cxp) / rx, (y1 - cyp) / ry)
    let dth = ang((x1 - cxp) / rx, (y1 - cyp) / ry, (-x1 - cxp) / rx, (-y1 - cyp) / ry)
    if (!fs && dth > 0) dth -= 2 * Math.PI
    if (fs && dth < 0) dth += 2 * Math.PI
    const n = Math.ceil(Math.abs(dth) / (Math.PI / 2) - 1e-9), step = dth / n
    const k = (4 / 3) * Math.tan(step / 4)
    for (let s = 0; s < n; s++) {
      const a1 = th1 + s * step, a2 = a1 + step
      const pt = (a, ox, oy) => [cx + rx * Math.cos(a) * cp - ry * Math.sin(a) * sp + ox, cy + rx * Math.cos(a) * sp + ry * Math.sin(a) * cp + oy]
      const d1 = (a) => [-rx * Math.sin(a) * cp - ry * Math.cos(a) * sp, -rx * Math.sin(a) * sp + ry * Math.cos(a) * cp]
      const [p1x, p1y] = pt(a1, 0, 0), [p2x, p2y] = pt(a2, 0, 0)
      const [t1x, t1y] = d1(a1), [t2x, t2y] = d1(a2)
      C(p1x + k * t1x, p1y + k * t1y, p2x - k * t2x, p2y - k * t2y, s === n - 1 ? ex : p2x, s === n - 1 ? ey : p2y)
    }
  }
  while (i < tokens.length) {
    if (/[a-zA-Z]/.test(tokens[i])) cmd = tokens[i++]
    const rel = cmd === cmd.toLowerCase(), up = cmd.toUpperCase()
    const ox = rel ? x : 0, oy = rel ? y : 0
    if (up === 'M') { x = next() + ox; y = next() + oy; sx = x; sy = y; res.push(`M${num(x)} ${num(y)}`); cmd = rel ? 'l' : 'L'; lastC = lastQ = false }
    else if (up === 'L') { x = next() + ox; y = next() + oy; res.push(`L${num(x)} ${num(y)}`); lastC = lastQ = false }
    else if (up === 'H') { x = next() + ox; res.push(`L${num(x)} ${num(y)}`); lastC = lastQ = false }
    else if (up === 'V') { y = next() + oy; res.push(`L${num(x)} ${num(y)}`); lastC = lastQ = false }
    else if (up === 'C') { const a = next() + ox, b = next() + oy, c = next() + ox, e = next() + oy, f = next() + ox, g = next() + oy; C(a, b, c, e, f, g) }
    else if (up === 'S') { const a = lastC ? 2 * x - lcx : x, b = lastC ? 2 * y - lcy : y, c = next() + ox, e = next() + oy, f = next() + ox, g = next() + oy; C(a, b, c, e, f, g) }
    else if (up === 'Q' || up === 'T') {
      let qx, qy
      if (up === 'Q') { qx = next() + ox; qy = next() + oy } else { qx = lastQ ? 2 * x - lqx : x; qy = lastQ ? 2 * y - lqy : y }
      const ex = next() + ox, ey = next() + oy
      C(x + (2 / 3) * (qx - x), y + (2 / 3) * (qy - y), ex + (2 / 3) * (qx - ex), ey + (2 / 3) * (qy - ey), ex, ey)
      lqx = qx; lqy = qy; lastQ = true; lastC = false
    }
    else if (up === 'A') { const rx = next(), ry = next(), phi = next(), fa = flag(), fs = flag(), ex = next() + ox, ey = next() + oy; arc(rx, ry, phi, fa, fs, ex, ey); lastC = lastQ = false }
    else if (up === 'Z') { res.push('Z'); x = sx; y = sy; lastC = lastQ = false }
    else throw new Error(`unsupported path command ${cmd}`)
  }
  return res.join(' ')
}

function phosphorPaths(name, weight) {
  const src = readFileSync(join(defs, `${name}.es.js`), 'utf8')
  const at = src.indexOf(`"${weight}",`)
  if (at < 0) throw new Error(`${name} has no ${weight}`)
  const rest = src.slice(at + weight.length + 3)
  const end = rest.search(/\n  \],?\n/)
  const block = rest.slice(0, end < 0 ? undefined : end)
  const paths = [...block.matchAll(/\bd: "([^"]+)"/g)].map((m) => m[1])
  if (!paths.length) throw new Error(`${name} ${weight}: no paths`)
  return paths.map(normalise)
}

const lowerFirst = (s) => s[0].toLowerCase() + s.slice(1)
const entries = []
for (const [name, weight] of config.phosphor) {
  entries.push({ name, viewBox: 256, stroke: 0, paths: phosphorPaths(name, weight) })
}
for (const [name, icon] of Object.entries(config.custom)) {
  entries.push({ name, viewBox: icon.viewBox, stroke: icon.stroke, paths: icon.paths.map(normalise) })
}

const lines = [
  '// GENERATED by scripts/gen-icons.mjs from @phosphor-icons/react and scripts/icons.json. Do not edit.',
  '',
  '/// Every icon the native chrome draws, named as the web names it (`Gear`, not `gear`).',
  'enum CrowbarIcon: String, CaseIterable {',
  ...entries.map((e) => `  case ${lowerFirst(e.name)} = "${e.name}"`),
  '}',
  '',
  '/// Path data (absolute M, L, C, Z) in a `viewBox` square; `stroke` > 0 is a stroked icon.',
  'struct CrowbarIconArt {',
  '  let viewBox: Double',
  '  let stroke: Double',
  '  let paths: [String]',
  '}',
  '',
  'extension CrowbarIcon {',
  '  var art: CrowbarIconArt {',
  '    switch self {',
  ...entries.flatMap((e) => [
    `    case .${lowerFirst(e.name)}:`,
    `      CrowbarIconArt(viewBox: ${e.viewBox}, stroke: ${e.stroke}, paths: [${e.paths.map((p) => JSON.stringify(p)).join(', ')}])`,
  ]),
  '    }',
  '  }',
  '}',
  '',
]
const text = lines.join('\n')
if (process.argv.includes('--check')) {
  let current = ''
  try { current = readFileSync(out, 'utf8') } catch {}
  if (current !== text) { console.error('CrowbarIconData.swift is stale: run node scripts/gen-icons.mjs'); process.exit(1) }
  console.log('CrowbarIconData.swift is up to date')
} else {
  writeFileSync(out, text)
  console.log(`wrote ${out} (${entries.length} icons, ${text.split('\n').length} lines)`)
}
