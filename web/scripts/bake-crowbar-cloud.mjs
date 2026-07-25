// Bakes a real crowbar mesh into a static point cloud for <AsciiCrowbar />.
//
// The component renders a point cloud (positions + surface normals); this script
// is the OFFLINE step that produces that cloud from a real 3D model, so the
// runtime stays pure-Math with no 3D library and no mesh parsing on the client.
//
// Source mesh: "Crowbar" by CreativeTrio — Public Domain (CC0), no attribution
// required — via poly.pizza (https://poly.pizza/m/MkTjC7C7bN). CC0 is deliberate:
// we must not embed a copyrighted model (e.g. Valve's Half-Life crowbar).
//
// Pipeline: download GLB → collect world-space triangles → reorient so the long
// axis is vertical → area-weighted surface sampling (seeded, deterministic) →
// recentre + unit-scale → quantize (Int16 position, Int8 normal) → base64 → emit
// ascii-crowbar-cloud.ts. Re-run with:  node scripts/bake-crowbar-cloud.mjs
//
// Tunables via env:  N=<points>  SEED=<int>

import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const HERE = dirname(fileURLToPath(import.meta.url))
const GLB_URL = 'https://static.poly.pizza/8fbe677c-0063-48ff-8f00-b843c6ceb0e3.glb'
const CACHE = join(HERE, '.cache', 'crowbar.glb')
const OUT = join(HERE, '..', 'src', 'features', 'panes', 'components', 'ascii-crowbar-cloud.ts')
const N = Number(process.env.N || 15000)
const SEED = Number(process.env.SEED || 1337)

// ── GLB (glTF 2.0 binary) reader → world-space triangles ────────────────────
const CT = {
  5120: Int8Array,
  5121: Uint8Array,
  5122: Int16Array,
  5123: Uint16Array,
  5125: Uint32Array,
  5126: Float32Array,
}
const NC = { SCALAR: 1, VEC2: 2, VEC3: 3, VEC4: 4, MAT4: 16 }

function parseGlb(buf) {
  if (buf.toString('ascii', 0, 4) !== 'glTF') throw new Error('not a glb')
  let off = 12,
    json = null,
    bin = null
  while (off < buf.length) {
    const len = buf.readUInt32LE(off),
      type = buf.readUInt32LE(off + 4)
    const data = buf.subarray(off + 8, off + 8 + len)
    if (type === 0x4e4f534a) json = JSON.parse(data.toString('utf8'))
    else if (type === 0x004e4942) bin = data
    off += 8 + len
  }
  return { json, bin }
}
function accessorArray(gltf, bin, i) {
  const acc = gltf.accessors[i],
    bv = gltf.bufferViews[acc.bufferView]
  const comps = NC[acc.type],
    TA = CT[acc.componentType]
  const base = (bv.byteOffset || 0) + (acc.byteOffset || 0)
  const stride = bv.byteStride || comps * TA.BYTES_PER_ELEMENT
  const out = new Float32Array(acc.count * comps)
  for (let k = 0; k < acc.count; k++) {
    const view = new TA(bin.buffer, bin.byteOffset + base + k * stride, comps)
    for (let c = 0; c < comps; c++) out[k * comps + c] = view[c]
  }
  return { data: out, count: acc.count }
}
function mul(a, b) {
  const o = new Array(16).fill(0)
  for (let c = 0; c < 4; c++)
    for (let r = 0; r < 4; r++)
      for (let k = 0; k < 4; k++) o[c * 4 + r] += a[k * 4 + r] * b[c * 4 + k]
  return o
}
function compose(t, r, s) {
  const [x, y, z, w] = r,
    x2 = x + x,
    y2 = y + y,
    z2 = z + z
  const xx = x * x2,
    xy = x * y2,
    xz = x * z2,
    yy = y * y2,
    yz = y * z2,
    zz = z * z2
  const wx = w * x2,
    wy = w * y2,
    wz = w * z2,
    [sx, sy, sz] = s
  return [
    (1 - (yy + zz)) * sx,
    (xy + wz) * sx,
    (xz - wy) * sx,
    0,
    (xy - wz) * sy,
    (1 - (xx + zz)) * sy,
    (yz + wx) * sy,
    0,
    (xz + wy) * sz,
    (yz - wx) * sz,
    (1 - (xx + yy)) * sz,
    0,
    t[0],
    t[1],
    t[2],
    1,
  ]
}
const nodeMatrix = (n) =>
  n.matrix
    ? n.matrix.slice()
    : compose(n.translation || [0, 0, 0], n.rotation || [0, 0, 0, 1], n.scale || [1, 1, 1])
const applyP = (m, x, y, z) => [
  m[0] * x + m[4] * y + m[8] * z + m[12],
  m[1] * x + m[5] * y + m[9] * z + m[13],
  m[2] * x + m[6] * y + m[10] * z + m[14],
]
const applyD = (m, x, y, z) => [
  m[0] * x + m[4] * y + m[8] * z,
  m[1] * x + m[5] * y + m[9] * z,
  m[2] * x + m[6] * y + m[10] * z,
]

function collectTriangles({ json, bin }) {
  const tris = []
  const scene = json.scenes[json.scene || 0]
  const walk = (ni, parent) => {
    const node = json.nodes[ni],
      world = mul(parent, nodeMatrix(node))
    if (node.mesh != null) {
      for (const prim of json.meshes[node.mesh].primitives) {
        if (prim.mode != null && prim.mode !== 4) continue
        const pos = accessorArray(json, bin, prim.attributes.POSITION)
        const nrm =
          prim.attributes.NORMAL != null ? accessorArray(json, bin, prim.attributes.NORMAL) : null
        let idx
        if (prim.indices != null) idx = accessorArray(json, bin, prim.indices).data
        else {
          idx = new Float32Array(pos.count)
          for (let i = 0; i < pos.count; i++) idx[i] = i
        }
        for (let t = 0; t < idx.length; t += 3) {
          const tri = []
          for (let k = 0; k < 3; k++) {
            const vi = idx[t + k]
            const p = applyP(world, pos.data[vi * 3], pos.data[vi * 3 + 1], pos.data[vi * 3 + 2])
            let n = null
            if (nrm) {
              n = applyD(world, nrm.data[vi * 3], nrm.data[vi * 3 + 1], nrm.data[vi * 3 + 2])
              const l = Math.hypot(n[0], n[1], n[2]) || 1
              n = [n[0] / l, n[1] / l, n[2] / l]
            }
            tri.push({ p, n })
          }
          tris.push(tri)
        }
      }
    }
    for (const c of node.children || []) walk(c, world)
  }
  const I = [1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]
  for (const n of scene.nodes) walk(n, I)
  return tris
}

// ── bake ────────────────────────────────────────────────────────────────────
function mulberry32(a) {
  return function () {
    a |= 0
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}
// Model long axis is +Z; Rx(-90) maps it to +Y so it stands vertical at rest.
const reorient = (p) => [p[0], p[2], -p[1]]

function bake(tris) {
  tris = tris.map((tri) => tri.map((v) => ({ p: reorient(v.p), n: v.n ? reorient(v.n) : null })))
  const cum = []
  let total = 0
  for (const tri of tris) {
    const [a, b, c] = tri.map((v) => v.p)
    const ux = b[0] - a[0],
      uy = b[1] - a[1],
      uz = b[2] - a[2]
    const vx = c[0] - a[0],
      vy = c[1] - a[1],
      vz = c[2] - a[2]
    const cx = uy * vz - uz * vy,
      cy = uz * vx - ux * vz,
      cz = ux * vy - uy * vx
    total += 0.5 * Math.hypot(cx, cy, cz)
    cum.push(total)
  }
  const rnd = mulberry32(SEED)
  const pick = () => {
    const r = rnd() * total
    let lo = 0,
      hi = cum.length - 1
    while (lo < hi) {
      const m = (lo + hi) >> 1
      if (cum[m] < r) lo = m + 1
      else hi = m
    }
    return lo
  }
  const pts = new Float32Array(N * 6)
  for (let i = 0; i < N; i++) {
    const [a, b, c] = tris[pick()]
    const su = Math.sqrt(rnd()),
      r2 = rnd()
    const w0 = 1 - su,
      w1 = su * (1 - r2),
      w2 = su * r2
    let nx, ny, nz
    if (a.n) {
      nx = a.n[0] * w0 + b.n[0] * w1 + c.n[0] * w2
      ny = a.n[1] * w0 + b.n[1] * w1 + c.n[1] * w2
      nz = a.n[2] * w0 + b.n[2] * w1 + c.n[2] * w2
    } else {
      const ux = b.p[0] - a.p[0],
        uy = b.p[1] - a.p[1],
        uz = b.p[2] - a.p[2]
      const vx = c.p[0] - a.p[0],
        vy = c.p[1] - a.p[1],
        vz = c.p[2] - a.p[2]
      nx = uy * vz - uz * vy
      ny = uz * vx - ux * vz
      nz = ux * vy - uy * vx
    }
    const l = Math.hypot(nx, ny, nz) || 1
    pts[i * 6] = a.p[0] * w0 + b.p[0] * w1 + c.p[0] * w2
    pts[i * 6 + 1] = a.p[1] * w0 + b.p[1] * w1 + c.p[1] * w2
    pts[i * 6 + 2] = a.p[2] * w0 + b.p[2] * w1 + c.p[2] * w2
    pts[i * 6 + 3] = nx / l
    pts[i * 6 + 4] = ny / l
    pts[i * 6 + 5] = nz / l
  }
  // recentre on bbox centre, scale to unit radius
  const lo = [1e9, 1e9, 1e9],
    hi = [-1e9, -1e9, -1e9]
  for (let i = 0; i < N; i++)
    for (let k = 0; k < 3; k++) {
      const v = pts[i * 6 + k]
      if (v < lo[k]) lo[k] = v
      if (v > hi[k]) hi[k] = v
    }
  const ctr = [(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2, (lo[2] + hi[2]) / 2]
  let maxR = 0
  for (let i = 0; i < N; i++) {
    const dx = pts[i * 6] - ctr[0],
      dy = pts[i * 6 + 1] - ctr[1],
      dz = pts[i * 6 + 2] - ctr[2]
    const r = dx * dx + dy * dy + dz * dz
    if (r > maxR) maxR = r
  }
  const scale = 1 / (Math.sqrt(maxR) || 1)
  for (let i = 0; i < N; i++) {
    pts[i * 6] = (pts[i * 6] - ctr[0]) * scale
    pts[i * 6 + 1] = (pts[i * 6 + 1] - ctr[1]) * scale
    pts[i * 6 + 2] = (pts[i * 6 + 2] - ctr[2]) * scale
  }
  return pts
}

function quantize(pts) {
  // interleaved per point: pos Int16 LE ×3, normal Int8 ×3 = 9 bytes
  const bytes = new Uint8Array(N * 9)
  const dv = new DataView(bytes.buffer)
  const clamp = (v, min, max) => (v < min ? min : v > max ? max : v)
  for (let i = 0; i < N; i++) {
    const o = i * 9
    dv.setInt16(o, clamp(Math.round(pts[i * 6] * 32767), -32768, 32767), true)
    dv.setInt16(o + 2, clamp(Math.round(pts[i * 6 + 1] * 32767), -32768, 32767), true)
    dv.setInt16(o + 4, clamp(Math.round(pts[i * 6 + 2] * 32767), -32768, 32767), true)
    dv.setInt8(o + 6, clamp(Math.round(pts[i * 6 + 3] * 127), -128, 127))
    dv.setInt8(o + 7, clamp(Math.round(pts[i * 6 + 4] * 127), -128, 127))
    dv.setInt8(o + 8, clamp(Math.round(pts[i * 6 + 5] * 127), -128, 127))
  }
  return Buffer.from(bytes).toString('base64')
}

async function getGlb() {
  if (existsSync(CACHE)) return readFileSync(CACHE)
  process.stderr.write(`Downloading ${GLB_URL}\n`)
  const res = await fetch(GLB_URL)
  if (!res.ok) throw new Error(`download failed: ${res.status}`)
  const buf = Buffer.from(await res.arrayBuffer())
  mkdirSync(dirname(CACHE), { recursive: true })
  writeFileSync(CACHE, buf)
  return buf
}

const glb = parseGlb(await getGlb())
const pts = bake(collectTriangles(glb))
const b64 = quantize(pts)

const ts = `// GENERATED by scripts/bake-crowbar-cloud.mjs — do not edit by hand.
//
// A crowbar surface baked to a point cloud for <AsciiCrowbar />: ${N} points,
// each a position (unit-scaled, recentred) + surface normal, quantized to
// Int16 position + Int8 normal and base64-encoded. Decoded once at module load.
//
// Source mesh: "Crowbar" by CreativeTrio — Public Domain (CC0), no attribution
// required — via poly.pizza (https://poly.pizza/m/MkTjC7C7bN).

const COUNT = ${N}

// prettier-ignore
const DATA =
  '${b64}'

let cache: { geo: Float32Array; count: number } | null = null

/** Decode the baked cloud into interleaved [x,y,z, nx,ny,nz] × COUNT. Cached. */
export function decodeCrowbarCloud(): { geo: Float32Array; count: number } {
  if (cache) return cache
  const bin = atob(DATA)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  const dv = new DataView(bytes.buffer)
  const geo = new Float32Array(COUNT * 6)
  for (let i = 0; i < COUNT; i++) {
    const o = i * 9
    geo[i * 6] = dv.getInt16(o, true) / 32767
    geo[i * 6 + 1] = dv.getInt16(o + 2, true) / 32767
    geo[i * 6 + 2] = dv.getInt16(o + 4, true) / 32767
    geo[i * 6 + 3] = dv.getInt8(o + 6) / 127
    geo[i * 6 + 4] = dv.getInt8(o + 7) / 127
    geo[i * 6 + 5] = dv.getInt8(o + 8) / 127
  }
  cache = { geo, count: COUNT }
  return cache
}
`

writeFileSync(OUT, ts)
process.stderr.write(
  `Wrote ${OUT}\n  points=${N} base64=${b64.length}B (~${(b64.length / 1024).toFixed(0)}KB)\n`,
)
