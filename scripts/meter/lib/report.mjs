import { homedir, tmpdir } from 'node:os'
import { realpathSync } from 'node:fs'
import { basename, dirname } from 'node:path'

const WRITE_FLAGS = 0x1 | 0x2 | 0x200 | 0x400 // O_WRONLY | O_RDWR | O_CREAT | O_TRUNC
const O_EVTONLY = 0x8000 // a kqueue watch (fsnotify), not a read
const O_DIRECTORY = 0x100000
const FS_FLAGS = { 0x100: 'created', 0x200: 'removed', 0x800: 'renamed', 0x1000: 'modified' }
// git's global options that take a separate value, skipped to find the subcommand.
const GIT_VALUE_OPTS = new Set(['-C', '-c', '--git-dir', '--work-tree', '--namespace'])

const bump = (map, key, by = 1) => {
  map[key] = (map[key] ?? 0) + by
}
const top = (map, n) =>
  Object.entries(map)
    .sort((a, b) => b[1] - a[1])
    .slice(0, n)

/**
 * Process identity across the whole trace: pid -> executable, from each
 * interposer `load` line and each exec. A PATH shim's exec of the real binary
 * is a hop of the exec that started the shim, not a second spawn. An exec is
 * `interposed` when the image it started logged its own `load`.
 */
export function processImages(trace, shimDir) {
  const images = new Map()
  const hops = new Set()
  const interposed = new Set()
  const pending = new Map()
  for (const l of trace) {
    if (l.call === 'load') {
      const exec = pending.get(l.pid)
      if (exec && basename(exec.path) === basename(l.path)) interposed.add(exec)
      pending.delete(l.pid)
      images.set(l.pid, l.path)
    } else if (l.call === 'execve') {
      if (shimDir && images.get(l.pid)?.startsWith(shimDir)) hops.add(l)
      images.set(l.pid, l.path)
      pending.set(l.pid, l)
    } else if (l.call === 'spawn' && l.result > 0) {
      images.set(l.result, l.path)
      pending.set(l.result, l)
    }
  }
  return { images, hops, interposed, shimDir }
}

/** Groups paths into what they are to Crowbar: its home, the fixture's git dir and worktrees, the system. */
export function categorizer(roots) {
  const rules = [
    [`${roots.repoDir}/.git/`, (rest) => `git-dir/${rest.split('/')[0]}`],
    [`${roots.feature}/`, () => 'worktree:feature'],
    [`${roots.base}/`, () => 'worktree:root'],
    [`${roots.repoDir}/`, () => 'repo-checkout'],
    [`${roots.sibling}/`, () => 'worktree:sibling'],
    [`${roots.origin}/`, () => 'origin'],
    [`${roots.home}/`, (rest) => `crowbar-home/${rest.split('/')[0]}`],
    [`${roots.runDir}/`, (rest) => `run/${rest.split('/')[0]}`],
    [`${realpathSync(tmpdir())}/`, () => 'tmp'],
    [`${homedir()}/`, (rest) => `user-home/${rest.split('/').slice(0, 2).join('/')}`],
    ['/dev/', () => 'dev'],
  ]
  return (path) => {
    if (!path) return 'unknown'
    for (const [prefix, name] of rules)
      if (`${path}/`.startsWith(prefix)) return name(path.slice(prefix.length))
    if (/^\/(System|usr|Library|private\/etc|etc|bin|sbin|opt|Applications)\//.test(path))
      return `system/${path.split('/')[1]}`
    return `other/${path.split('/')[1] ?? ''}`
  }
}

function execKey(path, argv) {
  const name = basename(path)
  const args = (argv ?? '').split(' ').slice(1)
  if (name === 'git') {
    for (let i = 0; i < args.length; i++) {
      if (GIT_VALUE_OPTS.has(args[i])) i++
      else if (!args[i].startsWith('-')) return `git ${args[i]}`
    }
    return 'git'
  }
  if (name === 'gh' || name === 'glab') return `${name} ${args.slice(0, 2).join(' ')}`.trim()
  return name
}

function binaryKey(path, shimDir) {
  if (shimDir && path.startsWith(shimDir)) return `${basename(path)} (via PATH shim)`
  if (path === '/usr/bin/git') return 'git (/usr/bin xcrun shim)'
  if (basename(path) === 'git') return `git (${dirname(path)})`
  return basename(path)
}

function processLabel(images, pid) {
  const image = images.get(pid)
  return image ? basename(image) : `pid ${pid}`
}

function openKind(flags) {
  if (flags & O_EVTONLY) return 'watch'
  if (flags & O_DIRECTORY) return 'dir'
  return flags & WRITE_FLAGS ? 'write' : 'read'
}

/** Counts for one window [from, to] (ms timestamps) of the trace and the FSEvents stream. */
export function summarize(trace, proc, { from, to, category, fsevents }) {
  const execs = { total: 0, byBinary: {}, byCommand: {}, byParent: {} }
  const opens = { ok: 0, failed: 0, byKind: {}, byProcess: {}, byCategory: {}, topDirs: {} }
  const changes = { writeOpens: 0, renames: 0, unlinks: 0, mkdirs: 0, byCategory: {} }

  for (const l of trace) {
    if (l.ts < from || l.ts > to) continue
    if (l.call === 'execve' || l.call === 'spawn') {
      if (l.call === 'spawn' && l.result <= 0) continue
      if (proc.hops.has(l)) continue
      const parent = processLabel(proc.images, l.call === 'spawn' ? l.pid : l.ppid)
      execs.total++
      bump(execs.byBinary, binaryKey(l.path, proc.shimDir))
      bump(execs.byCommand, `${execKey(l.path, l.extra)} @${category(l.cwd)}`)
      bump(execs.byParent, parent)
      continue
    }
    const who = processLabel(proc.images, l.pid)
    if (l.call === 'open') {
      if (l.result < 0) {
        opens.failed++
        continue
      }
      opens.ok++
      bump(opens.byKind, openKind(l.flags))
      bump(opens.byProcess, who)
      bump(opens.byCategory, category(l.path))
      bump(opens.topDirs, dirname(l.path))
      // /dev/null opened for writing is a child's stdio, not a file change.
      if (l.flags & WRITE_FLAGS && !l.path.startsWith('/dev/')) {
        changes.writeOpens++
        bump(changes.byCategory, category(l.path))
      }
      continue
    }
    if (l.result < 0) continue
    if (l.call === 'rename') changes.renames++
    else if (l.call === 'unlink') changes.unlinks++
    else if (l.call === 'mkdir') changes.mkdirs++
    else continue
    bump(changes.byCategory, category(l.path))
  }

  const fs = { total: 0, byCategory: {}, byKind: {} }
  for (const e of fsevents) {
    fs.total++
    bump(fs.byCategory, category(e.path))
    for (const [bit, kind] of Object.entries(FS_FLAGS))
      if (e.flags & Number(bit)) bump(fs.byKind, kind)
  }
  opens.topDirs = top(opens.topDirs, 12)
  return { execs, opens, changes, fsevents: fs }
}

const fmt = (entries) => entries.map(([k, v]) => `${k} ${v}`).join(', ') || '-'

export function printScenario(name, s, run) {
  const out = []
  const pad = (label) => label.padEnd(14)
  const quiet =
    run.quietMs === null
      ? 'never quiet for 3s'
      : `first quiet 3s at +${(run.quietMs / 1000).toFixed(1)}s`
  out.push(
    `\n== ${name}: ${run.seconds.toFixed(1)}s window (action ${run.actionSeconds.toFixed(1)}s, then observed; ${quiet})`,
  )
  out.push(`${pad('execs')}${s.execs.total}   by binary: ${fmt(top(s.execs.byBinary, 8))}`)
  out.push(`${pad('')}by command: ${fmt(top(s.execs.byCommand, 12))}`)
  out.push(`${pad('')}by parent: ${fmt(top(s.execs.byParent, 6))}`)
  out.push(
    `${pad('opens')}${s.opens.ok} ok, ${s.opens.failed} failed   by kind: ${fmt(top(s.opens.byKind, 4))}   by process: ${fmt(top(s.opens.byProcess, 6))}`,
  )
  out.push(`${pad('')}by category: ${fmt(top(s.opens.byCategory, 10))}`)
  out.push(
    `${pad('fds (daemon)')}${run.fds.start.total} -> ${run.fds.end.total}   end: ${fmt(top(run.fds.end.byType, 6))}`,
  )
  out.push(
    `${pad('file changes')}interposed: ${s.changes.writeOpens} write-opens, ${s.changes.renames} renames, ${s.changes.unlinks} unlinks, ${s.changes.mkdirs} mkdirs   by category: ${fmt(top(s.changes.byCategory, 8))}`,
  )
  out.push(
    `${pad('')}fsevents: ${s.fsevents.total} (${fmt(top(s.fsevents.byKind, 4))})   by category: ${fmt(top(s.fsevents.byCategory, 8))}`,
  )
  console.log(out.join('\n'))
}

export function printSummary(results) {
  const cols = [
    'scenario',
    'secs',
    'execs',
    'git',
    'gh',
    'opens',
    'opens/s',
    'writes',
    'fsevents',
    'fds end',
  ]
  const rows = results.map(({ name, run, s }) => {
    const count = (name) =>
      Object.entries(s.execs.byBinary)
        .filter(([k]) => k === name || k.startsWith(`${name} `))
        .reduce((n, [, v]) => n + v, 0)
    const writes = s.changes.writeOpens + s.changes.renames + s.changes.unlinks + s.changes.mkdirs
    return [
      name,
      run.seconds.toFixed(0),
      s.execs.total,
      count('git'),
      count('gh'),
      s.opens.ok,
      (s.opens.ok / run.seconds).toFixed(1),
      writes,
      s.fsevents.total,
      run.fds.end.total,
    ].map(String)
  })
  const widths = cols.map((c, i) => Math.max(c.length, ...rows.map((r) => r[i].length)))
  const line = (r) => r.map((v, i) => (i ? v.padStart(widths[i]) : v.padEnd(widths[i]))).join('  ')
  console.log(`\n${line(cols)}\n${widths.map((w) => '-'.repeat(w)).join('  ')}`)
  for (const r of rows) console.log(line(r))
}
