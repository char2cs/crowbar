import { execFileSync, spawn, spawnSync } from 'node:child_process'
import {
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { until } from '../../e2e/lib/wait.mjs'

const NATIVE = join(dirname(fileURLToPath(import.meta.url)), '../native')
export const SHIMMED = ['git', 'gh', 'glab', 'codex', 'curl']
// The executables whose processes (and descendants) the interposer logs.
const ROOTS = 'crowbar-api,crowbar-desktop'

function clang(args) {
  execFileSync('clang', ['-Wall', '-Wextra', '-O2', ...args], { stdio: 'inherit' })
}

/** Builds the interposer, shim and FSEvents observer into `dir` (never committed as binaries). */
export function buildNatives(dir) {
  mkdirSync(dir, { recursive: true })
  const out = {
    dylib: join(dir, 'interpose.dylib'),
    shim: join(dir, 'shim'),
    fsevents: join(dir, 'fsevents'),
    trace: join(dir, 'trace.log'),
    shimLog: join(dir, 'shim.log'),
    shimDir: join(dir, 'shims'),
  }
  clang(['-dynamiclib', '-o', out.dylib, join(NATIVE, 'interpose.c')])
  clang(['-o', out.shim, join(NATIVE, 'shim.c')])
  clang(['-framework', 'CoreServices', '-o', out.fsevents, join(NATIVE, 'fsevents.c')])
  writeFileSync(out.trace, '')
  return out
}

function script(path, body) {
  writeFileSync(path, `#!/bin/sh\n${body}`)
  chmodSync(path, 0o755)
  return path
}

/**
 * The variables the meter adds to the instance. DYLD_INSERT_LIBRARIES itself
 * would be dropped on the way (`bunx` starts the Tauri CLI through
 * /usr/bin/env, which SIP strips of DYLD_*), so cargo's runner sets it on the
 * app it runs; the sidecar and its children inherit it from there.
 */
export function meterEnv(natives, { shims }) {
  const triple = `${process.arch === 'arm64' ? 'aarch64' : 'x86_64'}-apple-darwin`
  const runner = script(
    join(dirname(natives.dylib), 'cargo-runner'),
    'DYLD_INSERT_LIBRARIES="$METER_DYLIB" exec "$@"\n',
  )
  const env = {
    [`CARGO_TARGET_${triple.toUpperCase().replace(/-/g, '_')}_RUNNER`]: runner,
    METER_DYLIB: natives.dylib,
    METER_LOG: natives.trace,
    METER_ROOTS: ROOTS,
  }
  if (!shims) return env
  mkdirSync(natives.shimDir, { recursive: true })
  for (const name of SHIMMED) symlinkSync(natives.shim, join(natives.shimDir, name))
  // The daemon replaces PATH with its login shell's, shims first only if that
  // shell reports them first: wrap the one probe it makes (shellenv).
  const shell = script(
    join(dirname(natives.shim), 'login-shell'),
    `if [ "$#" -eq 3 ] && [ "$1" = "-l" ] && [ "$2" = "-c" ] && [ "$3" = 'printf '"'"'%s'"'"' "$PATH"' ]; then
  out=$("$METER_REAL_SHELL" "$@") || exit $?
  printf '%s' "$METER_SHIM_DIR:$out"
  exit 0
fi
exec "$METER_REAL_SHELL" "$@"
`,
  )
  return {
    ...env,
    METER_SHIM_DIR: natives.shimDir,
    METER_SHIM_LOG: natives.shimLog,
    METER_REAL_SHELL: process.env.SHELL || '/bin/zsh',
    SHELL: shell,
  }
}

/** Parsed interposer log lines (see native/interpose.c for the format). */
export function readTrace(file) {
  const lines = []
  for (const raw of readFileSync(file, 'utf8').split('\n')) {
    if (!raw) continue
    const [ts, pid, ppid, call, result, flags, path, extra, cwd] = raw.split('\t')
    lines.push({
      ts: Number(ts),
      pid: Number(pid),
      ppid: Number(ppid),
      call,
      result: Number(result),
      flags: Number(flags),
      path,
      extra,
      cwd,
    })
  }
  return lines
}

export function readShimLog(file) {
  if (!existsSync(file)) return []
  return readFileSync(file, 'utf8')
    .split('\n')
    .filter(Boolean)
    .map((raw) => {
      const [ts, pid, ppid, name, argv = ''] = raw.split('\t')
      return { ts: Number(ts), pid: Number(pid), ppid: Number(ppid), name, argv }
    })
}

/**
 * Observes the traced processes for a fixed `ms` (a measured duration, so
 * windows compare across runs) and returns when they first went quiet for
 * `quietMs`, as ms after the start, or null if they never did.
 */
export async function observe(trace, ms, quietMs = 3000) {
  const started = Date.now()
  let size = statSync(trace).size
  let changed = started
  let quietAt = null
  await until(
    `the ${ms}ms observation window to end`,
    () => {
      const now = statSync(trace).size
      if (now !== size) {
        size = now
        changed = Date.now()
      }
      if (quietAt === null && Date.now() - changed >= quietMs) quietAt = changed - started
      return Date.now() - started >= ms
    },
    { timeout: ms + 10_000 },
  )
  return quietAt
}

/** Descriptors held by `pid`, by lsof TYPE. */
export function heldFds(pid) {
  const out = spawnSync('lsof', ['-n', '-P', '-F', 't', '-p', String(pid)], {
    encoding: 'utf8',
    maxBuffer: 64 * 1024 * 1024,
  }).stdout
  const byType = {}
  let total = 0
  for (const line of out.split('\n')) {
    if (!line.startsWith('t')) continue
    const type = line.slice(1)
    byType[type] = (byType[type] ?? 0) + 1
    total++
  }
  return { total, byType }
}

/** An FSEvents stream over `roots`, collecting every file event with its arrival time. */
export class FsObserver {
  constructor(bin, roots) {
    this.events = []
    this.ready = false
    this.proc = spawn(bin, roots, { stdio: ['ignore', 'pipe', 'inherit'] })
    let buf = ''
    this.proc.stdout.on('data', (chunk) => {
      buf += chunk
      const lines = buf.split('\n')
      buf = lines.pop()
      for (const line of lines) {
        if (line === 'ready') {
          this.ready = true
          continue
        }
        const [ts, flags, path] = line.split('\t')
        this.events.push({ ts: Number(ts), flags: Number(flags), path })
      }
    })
  }

  async start() {
    await until('the FSEvents observer to start', () => this.ready, { timeout: 10_000 })
  }

  /** Positive control: a file written under `dir` must be reported. */
  async proveSees(dir) {
    const canary = join(dir, `.meter-canary-${process.pid}`)
    writeFileSync(canary, 'canary\n')
    await until('the FSEvents observer to report the canary', () =>
      this.events.some((e) => e.path === canary),
    )
    return canary
  }

  between(from, to) {
    return this.events.filter((e) => e.ts >= from && e.ts <= to)
  }

  stop() {
    this.proc.kill('SIGTERM')
  }
}
