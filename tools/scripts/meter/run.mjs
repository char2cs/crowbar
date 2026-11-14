#!/usr/bin/env bun
// Entry point: `bun scripts/meter/run.mjs [--only=a,b] [--idle=310] [--tail=30] [--shims] [--keep]`
// (see README.md; `make meter` wraps it).
import { spawnSync } from 'node:child_process'
import { appendFileSync, mkdirSync, rmSync, unlinkSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  createChat,
  createWorkspaceChat,
  importFixture,
  waitStubProviderEnabled,
} from '../e2e/lib/fixture.mjs'
import { Instance } from '../e2e/lib/instance.mjs'
import { openChat, reload } from '../e2e/lib/ui.mjs'
import { advanceOrigin, createCloneFixture, git } from './lib/fixture.mjs'
import {
  FsObserver,
  SHIMMED,
  buildNatives,
  heldFds,
  meterEnv,
  readShimLog,
  readTrace,
  observe,
} from './lib/probes.mjs'
import {
  categorizer,
  printScenario,
  printSummary,
  processImages,
  summarize,
} from './lib/report.mjs'

const SCENARIOS = ['cold-open', 'idle', 'switch', 'burst', 'fetch']
const flags = Object.fromEntries(
  process.argv.slice(2).map((arg) => {
    const [key, value = 'true'] = arg.replace(/^--/, '').split('=')
    return [key, value]
  }),
)
const only = flags.only ? flags.only.split(',') : SCENARIOS
for (const name of only)
  if (!SCENARIOS.includes(name)) throw new Error(`unknown scenario ${name}; known: ${SCENARIOS}`)
// Every window is its action plus a fixed observation tail, so runs compare.
const IDLE_SECONDS = Number(flags.idle ?? 310)
const TAIL_SECONDS = Number(flags.tail ?? 30)
const shims = flags.shims === 'true'

/** The e2e instance, with the meter's variables in every process it starts. */
class MeteredInstance extends Instance {
  env(extra = {}) {
    const env = super.env({ ...this.meterEnv, ...extra })
    if (env.METER_SHIM_DIR) env.PATH = `${env.METER_SHIM_DIR}:${env.PATH}`
    return env
  }
}

function refuseSecondDevInstance() {
  const out = spawnSync('pgrep', ['-fl', 'crowbar-api|crowbar-desktop'], {
    encoding: 'utf8',
  }).stdout
  const dev = out.split('\n').filter((l) => l && !l.includes('/Applications/'))
  if (dev.length)
    throw new Error(`another dev instance is running (one at a time):\n${dev.join('\n')}`)
}

/** Opens a chat whose row sits inside a protected branch row, unfolding it first if folded. */
async function openNested(d, parentRowId, chatId) {
  const inRecents = (id) => !!document.querySelector(`[data-testid="recents-row-${id}"]`)
  if (!(await d.eval(inRecents, chatId))) {
    const folded = await d.until(
      'the protected branch row',
      (id) => {
        const row = document.querySelector(`[data-sidebar-row-id="${id}"]`)
        return row ? (row.hasAttribute('data-sidebar-expanded') ? 'open' : 'folded') : null
      },
      parentRowId,
    )
    if (folded === 'folded')
      await d.eval((id) => {
        document.querySelector(`[data-sidebar-row-id="${id}"]`).click()
        return true
      }, parentRowId)
  }
  await openChat(d, chatId)
}

async function focus(d, chat) {
  await d.until(
    'the Recents row to take the click',
    (id) => {
      const row = document.querySelector(`[data-testid="recents-row-${id}"] [role="treeitem"]`)
      row?.click()
      return !!row
    },
    chat.id,
  )
  await d.until(
    'the workspace to become active',
    (ws) => {
      return (
        document.querySelector('[data-workspace-slot][data-active="true"]')?.dataset
          .workspaceSlot === ws
      )
    },
    chat.workspaceId,
  )
}

// An editor-like burst: creates, edits of tracked files, deletes, ~50 ops over ~5s.
async function burst(worktree) {
  const dir = join(worktree, 'meter-burst')
  mkdirSync(dir, { recursive: true })
  const tracked = git(worktree, 'ls-files', 'docs')
    .split('\n')
    .filter((f) => f.endsWith('.md'))
    .slice(0, 10)
  const created = []
  for (let i = 0; i < 50; i++) {
    const op = i % 5
    if (op === 2 && tracked.length)
      appendFileSync(join(worktree, tracked[i % tracked.length]), `\nmeter ${i}\n`)
    else if (op === 3 && created.length) unlinkSync(created.shift())
    else {
      const sub = op === 4 ? join(dir, `sub${i}`) : dir
      mkdirSync(sub, { recursive: true })
      const file = join(sub, `f${i}.txt`)
      writeFileSync(file, `burst ${i}\n`)
      created.push(file)
    }
    // Load shape, not synchronization: the burst is spread like a human or agent edit.
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
}

/** workspace id -> worktree path, joined on branch with git's own worktree list. */
async function worktreePaths(daemon, e2e, repoDir) {
  const byBranch = {}
  let path
  for (const line of git(repoDir, 'worktree', 'list', '--porcelain').split('\n')) {
    if (line.startsWith('worktree ')) path = line.slice(9)
    else if (line.startsWith('branch refs/heads/')) byBranch[line.slice(18)] = path
  }
  const workspaces = await daemon.get(`${e2e.base}/workspaces`)
  return Object.fromEntries(workspaces.map((w) => [w.id, byBranch[w.branch]]))
}

const inst = new MeteredInstance('tauri')
const meterDir = join(inst.runDir, 'meter')
let exitCode = 0
let observer
try {
  refuseSecondDevInstance()
  console.log(`meter: run dir ${inst.runDir}`)
  const natives = buildNatives(meterDir)
  inst.meterEnv = meterEnv(natives, { shims })
  console.log('meter: cloning the fixture repo')
  const fx = createCloneFixture(inst.runDir)
  console.log(`meter: fixture ${fx.repoDir} (${fx.files} tracked files)`)
  mkdirSync(inst.home, { recursive: true })
  observer = new FsObserver(natives.fsevents, [inst.home, fx.projectDir, fx.sibling])
  await observer.start()
  await observer.proveSees(inst.home)

  await inst.start()
  await inst.launchApp({ shell: false })
  await inst.daemon.waitHealthy()
  const e2e = await importFixture(inst.daemon, { projectDir: fx.projectDir, repoDir: fx.repoDir })
  await waitStubProviderEnabled(inst.daemon, e2e)
  const feature = await createWorkspaceChat(inst.daemon, e2e)
  // The protected root opens no view of its own: a chat inside it does.
  const rootWs = (await inst.daemon.get(`${e2e.base}/chats/${e2e.baseChatId}`)).workspaceId
  const base = await createChat(inst.daemon, e2e, rootWs)
  const paths = await worktreePaths(inst.daemon, e2e, fx.repoDir)
  const basePath = paths[base.workspaceId]
  const featurePath = paths[feature.workspaceId]
  if (!basePath || !featurePath)
    throw new Error(`a workspace has no worktree: ${JSON.stringify(paths)}`)
  await reload(inst.driver, { home: true })
  const openViews = async () => {
    await openNested(inst.driver, e2e.baseChatId, base.id)
    await openChat(inst.driver, feature.id)
  }
  await openViews()
  console.log(`meter: workspaces root=${basePath} feature=${featurePath}`)

  const d = { eval: (...a) => inst.driver.eval(...a), until: (...a) => inst.driver.until(...a) }
  const results = []
  let daemonPid = await inst.daemonPid()
  let fds = heldFds(daemonPid)
  async function measure(name, action, tailSeconds = TAIL_SECONDS) {
    if (!only.includes(name)) return
    const from = Date.now()
    const start = fds
    await action()
    const actionSeconds = (Date.now() - from) / 1000
    const quietMs = await observe(natives.trace, tailSeconds * 1000)
    const to = Date.now()
    daemonPid = await inst.daemonPid()
    fds = heldFds(daemonPid)
    const run = {
      from,
      to,
      seconds: (to - from) / 1000,
      actionSeconds,
      quietMs,
      fds: { start, end: fds },
    }
    results.push({ name, run })
    console.log(`meter: ${name} measured (${run.seconds.toFixed(1)}s)`)
  }

  await measure('cold-open', async () => {
    fds = { total: 0, byType: {} }
    await inst.relaunchApp()
    await openViews()
  })
  await measure('idle', async () => {}, IDLE_SECONDS)
  await measure('switch', async () => {
    for (let i = 0; i < 10; i++) await focus(d, i % 2 ? base : feature)
  })
  await measure('burst', () => burst(featurePath))
  advanceOrigin(fx, `fetch ${Date.now()}`)
  await measure('fetch', async () => git(fx.sibling, 'fetch', '--quiet', 'origin'))

  // Everything is read after the run, so the meter's own parsing costs no window.
  const trace = readTrace(natives.trace)
  const proc = processImages(trace, shims ? natives.shimDir : null)
  const category = categorizer({
    runDir: inst.runDir,
    home: inst.home,
    repoDir: fx.repoDir,
    base: basePath,
    feature: featurePath,
    sibling: fx.sibling,
    origin: fx.origin,
  })
  const controls = positiveControls(trace, proc, natives, daemonPid, observer)
  for (const r of results) {
    r.s = summarize(trace, proc, {
      from: r.run.from,
      to: r.run.to,
      category,
      fsevents: observer.between(r.run.from, r.run.to),
    })
    printScenario(r.name, r.s, r.run)
  }
  printSummary(results)
  const report = join(meterDir, 'report.json')
  writeFileSync(
    report,
    JSON.stringify(
      {
        fixture: { files: fx.files, shims, idleSeconds: IDLE_SECONDS, tailSeconds: TAIL_SECONDS },
        controls,
        results,
      },
      null,
      2,
    ),
  )
  console.log(`\nmeter: report ${report}`)
  if (!controls.ok) exitCode = 1
} catch (err) {
  exitCode = 1
  console.log(`meter failed: ${err.stack ?? err}`)
} finally {
  observer?.stop()
  await inst.teardown({ keep: true })
  if (flags.keep !== 'true')
    for (const dir of ['project', 'remotes', 'sibling', 'pusher'])
      rmSync(join(inst.runDir, dir), { recursive: true, force: true })
  console.log(`meter: run dir kept at ${inst.runDir}`)
}
process.exit(exitCode)

/** Proves each counter observed the daemon: a meter reading zero because it never loaded is the classic failure. */
function positiveControls(trace, proc, natives, daemonPid, fsObserver) {
  const loads = trace.filter((l) => l.call === 'load')
  const daemonLoad = loads.find((l) => l.pid === daemonPid && l.path.endsWith('/crowbar-api'))
  const daemonOpens = trace.filter((l) => l.pid === daemonPid && l.call === 'open').length
  const children = {}
  for (const l of trace) {
    if (l.call !== 'execve' && !(l.call === 'spawn' && l.result > 0)) continue
    const c = (children[l.path] ??= { execs: 0, interposed: 0 })
    c.execs++
    if (proc.interposed.has(l)) c.interposed++
  }
  const shimLines = readShimLog(natives.shimLog)
  const shimCounts = Object.fromEntries(
    SHIMMED.map((n) => [n, shimLines.filter((l) => l.name === n).length]),
  )
  const fds = heldFds(daemonPid)
  const controls = {
    daemonPid,
    interposerInDaemon: !!daemonLoad,
    daemonOpens,
    desktopInterposed: loads.some((l) => l.path.endsWith('/crowbar-desktop')),
    children,
    lsofRegularFiles: fds.byType.REG ?? 0,
    fseventsCanary: fsObserver.events.some((e) => e.path.includes('.meter-canary-')),
    shims: shims ? shimCounts : 'off',
  }
  controls.ok =
    controls.interposerInDaemon &&
    daemonOpens > 0 &&
    controls.lsofRegularFiles > 0 &&
    controls.fseventsCanary &&
    (!shims || shimLines.length > 0)
  console.log('\n== positive controls')
  console.log(
    `interposer in daemon pid ${daemonPid}: ${controls.interposerInDaemon ? 'loaded' : 'NOT LOADED'}, ${daemonOpens} opens logged`,
  )
  console.log(`interposer in desktop app: ${controls.desktopInterposed ? 'loaded' : 'not loaded'}`)
  console.log('children (exec path: execs, interposed):')
  for (const [path, c] of Object.entries(children).sort((a, b) => b[1].execs - a[1].execs))
    console.log(`  ${path}: ${c.execs}, ${c.interposed}`)
  console.log(`lsof: daemon holds ${fds.total} fds (${fds.byType.REG ?? 0} REG)`)
  console.log(`fsevents canary: ${controls.fseventsCanary ? 'seen' : 'NOT SEEN'}`)
  if (shims) console.log(`shims: ${shimLines.length} calls logged ${JSON.stringify(shimCounts)}`)
  if (!controls.ok) console.log('POSITIVE CONTROL FAILED: the counts below are not trustworthy')
  return controls
}
