import { spawn, spawnSync } from 'node:child_process'
import {
  createWriteStream,
  existsSync,
  mkdtempSync,
  realpathSync,
  rmSync,
} from 'node:fs'
import { homedir, tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { CdpDriver } from './cdp.mjs'
import { daemonClient } from './daemon.mjs'
import { installStubProvider } from './stub-provider.mjs'
import { until } from './wait.mjs'

export const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../../..')
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

/** One isolated dev instance of this checkout: its own CROWBAR_HOME and Vite origin, never the
 *  production ~/.crowbar. It drives the bundle in a launched Chrome over CDP, against a daemon on
 *  loopback TCP. */
export class Instance {
  constructor() {
    // Created under the OS temp dir, never under the source checkout.
    this.runDir = realpathSync(mkdtempSync(join(tmpdir(), 'crowbar-e2e-')))
    this.home = join(this.runDir, 'home')
    this.devPort = freePort()
    this.apiPort = freePort()
    this.daemon = null
    this.driver = null
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

  /** Brings up the daemon and Vite; the browser is launched by launchApp(). */
  async start() {
    installStubProvider(this.home, this.runDir)
    this.sh(
      'go',
      ['build', '-tags', 'noEmbed', '-o', this.daemonBin(), './cmd/crowbar'],
      join(REPO_ROOT, 'api'),
    )
    this.daemon = daemonClient(`http://127.0.0.1:${this.apiPort}`)
    this.startDaemonProcess()
    this.vite = this.spawnLogged(
      'vite',
      'bunx',
      ['vite', '--port', String(this.devPort), '--strictPort'],
      join(REPO_ROOT, 'app/panes'),
      { VITE_API_URL: `http://127.0.0.1:${this.apiPort}` },
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

  async launchApp() {
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
    await this.daemon.waitHealthy()
    return this.openPage()
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

  /** Closes the browser without touching the daemon, Vite or the home. */
  async stopApp() {
    this.driver?.close()
    this.driver = null
    const proc = this.chrome
    this.chrome = null
    if (proc) {
      killGroup(proc, 'SIGTERM')
      await onceExited(proc)
    }
  }

  /** A new tab in the same browser: the process keeps its committed storage, as a reopened
   *  window would (a killed Chrome loses its last localStorage writes). */
  async relaunchApp() {
    await this.driver.closeTab()
    return this.openPage()
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
    this.startDaemonProcess()
    const pid = await until(
      'a new daemon to answer',
      async () => {
        const pid = await this.daemonPid()
        return pid && pid !== old ? pid : null
      },
      { timeout: 60_000 },
    )
    await this.daemon.waitHealthy()
    return pid
  }

  async teardown({ keep }) {
    const pid = await this.daemonPid().catch(() => null)
    try {
      await this.stopApp()
    } catch {
      if (this.chrome) killGroup(this.chrome, 'SIGKILL')
    }
    if (pid && alive(pid)) process.kill(pid, 'SIGTERM')
    if (this.vite) killGroup(this.vite, 'SIGTERM')
    if (!keep && existsSync(this.runDir)) rmSync(this.runDir, { recursive: true, force: true })
  }
}
