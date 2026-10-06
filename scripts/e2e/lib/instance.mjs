import { spawn, spawnSync } from 'node:child_process'
import {
  createWriteStream,
  existsSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
} from 'node:fs'
import { homedir, tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { CdpDriver } from './cdp.mjs'
import { daemonClient, socketPathFor } from './daemon.mjs'
import { Driver, Fatal } from './driver.mjs'
import { installStubProvider } from './stub-provider.mjs'
import { until } from './wait.mjs'

export const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const TOOL_PATH = [
  join(homedir(), '.bun/bin'),
  join(homedir(), '.cargo/bin'),
  join(homedir(), '.rustup/toolchains/stable-aarch64-apple-darwin/bin'),
  process.env.PATH,
].join(':')
const CHROME =
  process.env.CROWBAR_E2E_CHROME ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

function freePort() {
  const server = Bun.listen({
    hostname: '127.0.0.1',
    port: 0,
    socket: { data() {} },
  })
  const { port } = server
  server.stop(true)
  return port
}

function alive(pid) {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}

function killGroup(child, signal) {
  try {
    process.kill(-child.pid, signal)
  } catch {
    // already gone
  }
}

function onceExited(child) {
  if (child.exitCode !== null || child.signalCode !== null) return Promise.resolve()
  return new Promise((resolveExit) => {
    child.once('exit', resolveExit)
    const timer = setTimeout(() => killGroup(child, 'SIGKILL'), 15_000)
    child.once('exit', () => clearTimeout(timer))
  })
}

/**
 * One isolated dev instance of THIS checkout: its own CROWBAR_HOME (so its own
 * daemon, projects and logs), its own Vite origin (so its own localStorage and
 * IndexedDB) and a real UI to drive. It never reads or writes the production
 * ~/.crowbar.
 *
 * target 'tauri'   the real desktop app (debug build) with its daemon sidecar,
 *                  driven through the app's in-process bridge.
 * target 'browser' the same web bundle in a Chrome the suite launches, talking
 *                  to a daemon the suite launches on loopback TCP, driven over CDP.
 */
export class Instance {
  constructor(target) {
    this.target = target
    // Created under the OS temp dir, never under the source checkout.
    this.runDir = realpathSync(mkdtempSync(join(tmpdir(), 'crowbar-e2e-')))
    this.home = join(this.runDir, 'home')
    this.devPort = freePort()
    this.apiPort = target === 'browser' ? freePort() : 0
    this.daemon = daemonClient(
      target === 'browser'
        ? `http://127.0.0.1:${this.apiPort}`
        : socketPathFor(this.home, tmpdir()),
    )
    this.driver = null
    this.app = null
    this.vite = null
    this.daemonProc = null
    const prod = join(homedir(), '.crowbar')
    if (this.home === prod || this.home.startsWith(prod + '/')) {
      throw new Error('refusing to run against the production Crowbar home')
    }
  }

  env(extra = {}) {
    return {
      ...process.env,
      PATH: TOOL_PATH,
      CROWBAR_HOME: this.home,
      CROWBAR_DEV_LABEL: 'e2e',
      CROWBAR_DEV_PORT: String(this.devPort),
      ...extra,
    }
  }

  spawnLogged(name, command, args, cwd, extraEnv) {
    const log = createWriteStream(join(this.runDir, `${name}.log`))
    const child = spawn(command, args, {
      cwd,
      env: this.env(extraEnv),
      detached: true,
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    child.stdout.pipe(log)
    child.stderr.pipe(log)
    return child
  }

  sh(command, args, cwd) {
    const done = spawnSync(command, args, {
      cwd,
      env: this.env(),
      encoding: 'utf8',
    })
    if (done.status !== 0) throw new Error(`${command} ${args.join(' ')} failed:\n${done.stderr}`)
  }

  /** Brings up the daemon (browser target) and Vite; the app itself is launched by launchApp(). */
  async start() {
    installStubProvider(this.home, this.runDir)
    if (this.target === 'browser') {
      this.sh(
        'go',
        ['build', '-tags', 'noEmbed', '-o', this.daemonBin(), './cmd/crowbar'],
        join(REPO_ROOT, 'api'),
      )
      this.startDaemonProcess()
    } else {
      this.sh('make', ['-C', 'desktop', 'fetch-sidecar'], REPO_ROOT)
    }
    this.vite = this.spawnLogged(
      'vite',
      'bunx',
      ['vite', '--port', String(this.devPort), '--strictPort'],
      join(REPO_ROOT, 'web'),
      this.target === 'browser' ? { VITE_API_URL: `http://127.0.0.1:${this.apiPort}` } : {},
    )
    await until(
      'the Vite dev server',
      async () => (await fetch(`http://localhost:${this.devPort}/`)).ok,
      {
        timeout: 60_000,
      },
    )
  }

  daemonBin() {
    return join(this.runDir, 'crowbar-daemon')
  }

  startDaemonProcess() {
    this.daemonProc = this.spawnLogged(
      `daemon-${Date.now()}`,
      this.daemonBin(),
      ['serve', '--host', `tcp://127.0.0.1:${this.apiPort}`],
      this.runDir,
    )
  }

  async launchApp({ shell = true } = {}) {
    if (this.target === 'browser') {
      this.chrome = this.spawnLogged(
        `chrome-${Date.now()}`,
        CHROME,
        [
          '--headless=new',
          `--remote-debugging-port=${(this.debugPort ??= freePort())}`,
          `--user-data-dir=${join(this.runDir, 'chrome')}`,
          '--window-size=1500,950',
          '--no-first-run',
          '--disable-background-timer-throttling',
          'about:blank',
        ],
        this.runDir,
      )
      this.driver = null
    } else {
      // The app is rebuilt on each start (the sidecar binary changes), and cargo
      // occasionally fails an archive step; one retry absorbs that, a second failure is real.
      for (let attempt = 1; ; attempt++) {
        try {
          await this.startTauri()
          break
        } catch (err) {
          if (!(err instanceof Fatal) || attempt >= 2) throw err
          console.log(`  app build failed (${err.message}); retrying once`)
          killGroup(this.app, 'SIGKILL')
        }
      }
    }
    await this.daemon.waitHealthy()
    if (this.target === 'browser') return this.openPage()
    if (!shell) return
    await this.waitShell()
  }

  async startTauri() {
    const config = JSON.stringify({
      build: { devUrl: `http://localhost:${this.devPort}`, beforeDevCommand: '' },
    })
    const name = `app-${Date.now()}`
    this.app = this.spawnLogged(
      name,
      'bunx',
      ['@tauri-apps/cli', 'dev', '--no-watch', '--config', config],
      join(REPO_ROOT, 'desktop'),
    )
    const log = join(this.runDir, `${name}.log`)
    this.driver = await Driver.attach(this.devPort, {
      timeout: 600_000,
      // A failed build never opens a bridge: stop waiting for one.
      failed: () => {
        const text = existsSync(log) ? readFileSync(log, 'utf8') : ''
        return text.includes('could not compile') ? 'cargo could not compile the app' : null
      },
    })
  }

  async openPage() {
    this.driver = await CdpDriver.attach(this.debugPort, `http://localhost:${this.devPort}/`)
    await this.waitShell()
  }

  async waitShell() {
    await this.driver.until(
      'the app shell to mount',
      () => !!document.querySelector('[data-slot="console-dock"]'),
      null,
      { timeout: 60_000 },
    )
  }

  /** Closes the whole app (window and, on Tauri, its sidecar daemon) without touching Vite or the home. */
  async stopApp() {
    this.driver?.close()
    this.driver = null
    const proc = this.target === 'browser' ? this.chrome : this.app
    this.chrome = this.app = null
    if (proc) {
      killGroup(proc, 'SIGTERM')
      await onceExited(proc)
    }
    if (this.target === 'tauri') {
      const pid = this.lastDaemonPid
      if (pid && alive(pid)) {
        try {
          await until('the daemon to exit with the app', () => !alive(pid), {
            timeout: 5000,
          })
        } catch {
          process.kill(pid, 'SIGTERM')
        }
      }
    }
  }

  async relaunchApp() {
    if (this.target === 'browser') {
      // New tab in the same browser: the process keeps its committed storage, as a
      // reopened window would. Only the desktop target restarts the whole process.
      await this.driver.closeTab()
      return this.openPage()
    }
    this.lastDaemonPid = await this.daemonPid()
    await this.stopApp()
    if (this.target === 'tauri') await this.waitDaemonGone()
    await this.launchApp()
  }

  async waitDaemonGone() {
    if (!this.lastDaemonPid) return
    await until('the old daemon to exit', () => !alive(this.lastDaemonPid), {
      timeout: 20_000,
    })
  }

  async daemonPid() {
    return (await this.daemon.health())?.data?.pid ?? null
  }

  /** Kills the daemon and waits for a NEW one to answer. Returns the new pid. */
  async restartDaemon() {
    const old = await this.daemonPid()
    process.kill(old, 'SIGTERM')
    await until('the old daemon to exit', () => !alive(old), {
      timeout: 20_000,
    })
    if (this.target === 'browser') this.startDaemonProcess()
    return until(
      'a new daemon to answer',
      async () => {
        const pid = await this.daemonPid()
        return pid && pid !== old ? pid : null
      },
      { timeout: 60_000 },
    )
  }

  async teardown({ keep }) {
    this.lastDaemonPid = await this.daemonPid().catch(() => null)
    try {
      await this.stopApp()
    } catch {
      for (const proc of [this.app, this.chrome]) if (proc) killGroup(proc, 'SIGKILL')
    }
    const pid = this.lastDaemonPid
    if (pid && alive(pid)) process.kill(pid, 'SIGTERM')
    if (this.vite) killGroup(this.vite, 'SIGTERM')
    if (!keep && existsSync(this.runDir)) rmSync(this.runDir, { recursive: true, force: true })
  }
}
