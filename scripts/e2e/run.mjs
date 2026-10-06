#!/usr/bin/env bun
// Entry point: `bun scripts/e2e/run.mjs [--target=tauri|browser] [--only=a,b] [--keep]`
// (see README.md; `make e2e` wraps it).
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { createGitFixture, importFixture, waitStubProviderEnabled } from './lib/fixture.mjs'
import { Instance, REPO_ROOT } from './lib/instance.mjs'
import { closeAllViews, reload } from './lib/ui.mjs'
import console_ from './scenarios/console.mjs'
import recents from './scenarios/recents.mjs'
import closeView from './scenarios/close-view.mjs'
import subagent from './scenarios/subagent.mjs'
import codexOrder from './scenarios/codex-order.mjs'
import workingLine from './scenarios/working-line.mjs'
import permissionFrame from './scenarios/permission-frame.mjs'
import steer from './scenarios/steer.mjs'
import workspaceSwitch from './scenarios/workspace-switch.mjs'
import sidebarBand from './scenarios/sidebar-band.mjs'

const SCENARIOS = [
  console_,
  recents,
  closeView,
  subagent,
  codexOrder,
  workingLine,
  permissionFrame,
  steer,
  workspaceSwitch,
  sidebarBand,
]

const flags = Object.fromEntries(
  process.argv.slice(2).map((arg) => {
    const [key, value = 'true'] = arg.replace(/^--/, '').split('=')
    return [key, value]
  }),
)
const target = flags.target ?? 'tauri'
if (!['tauri', 'browser'].includes(target)) throw new Error(`unknown --target=${target}`)
const only = flags.only ? flags.only.split(',') : null
const selected = SCENARIOS.filter((s) => !only || only.includes(s.name))
if (only && selected.length !== only.length) {
  throw new Error(`unknown scenario in --only; known: ${SCENARIOS.map((s) => s.name).join(', ')}`)
}

const artifacts = join(
  REPO_ROOT,
  'scripts/e2e/artifacts',
  new Date().toISOString().replace(/[:.]/g, '-'),
)
const inst = new Instance(target)
const results = []
let exitCode = 0

try {
  console.log(`e2e: target=${target} home=${inst.home} origin=http://localhost:${inst.devPort}`)
  await inst.start()
  // The desktop app owns its daemon, so it must start first. The browser
  // target's daemon is already up. Either way the project is seeded BEFORE the
  // shell is shown: a window with no project lands on onboarding.
  if (target === 'tauri') await inst.launchApp({ shell: false })
  await inst.daemon.waitHealthy()
  const fx = await importFixture(inst.daemon, createGitFixture(inst.runDir))
  await waitStubProviderEnabled(inst.daemon, fx)
  if (target === 'tauri') await reload(inst.driver, { home: true })
  else await inst.launchApp()
  const ctx = {
    inst,
    fx,
    target,
    daemon: inst.daemon,
    // A stable handle: the underlying driver is replaced when the app relaunches.
    d: {
      eval: (...args) => inst.driver.eval(...args),
      until: (...args) => inst.driver.until(...args),
      emit: (...args) => inst.driver.emit(...args),
    },
  }
  for (const scenario of selected) {
    const started = Date.now()
    try {
      await scenario.run(ctx)
      results.push({ name: scenario.name, ok: true, ms: Date.now() - started })
      console.log(`  PASS ${scenario.name} (${Date.now() - started}ms)`)
    } catch (err) {
      exitCode = 1
      results.push({
        name: scenario.name,
        ok: false,
        ms: Date.now() - started,
      })
      console.log(`  FAIL ${scenario.name}\n${String(err.stack ?? err).replace(/^/gm, '    ')}`)
      mkdirSync(artifacts, { recursive: true })
      await inst.driver
        ?.screenshot(join(artifacts, `${scenario.name}.png`))
        .catch((e) => console.log(`    (no screenshot: ${e.message})`))
    }
    await closeAllViews(ctx.d).catch(() => {})
  }
} catch (err) {
  exitCode = 1
  console.log(`e2e setup failed: ${err.stack ?? err}`)
  mkdirSync(artifacts, { recursive: true })
  await inst.driver?.screenshot(join(artifacts, 'setup.png')).catch(() => {})
} finally {
  await inst.teardown({ keep: exitCode !== 0 || flags.keep === 'true' })
}

const failed = results.filter((r) => !r.ok).length
console.log(
  `e2e: ${results.length - failed}/${results.length} scenarios passed${exitCode ? ` (logs and screenshots kept: ${inst.runDir}, ${artifacts})` : ''}`,
)
process.exit(exitCode)
