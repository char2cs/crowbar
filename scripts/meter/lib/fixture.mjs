import { execFileSync } from 'node:child_process'
import { appendFileSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { REPO_ROOT } from '../../e2e/lib/instance.mjs'

export function git(cwd, ...args) {
  // Pinned to the fixture: no inherited GIT_* can point git at another repository.
  const env = { ...process.env, GIT_CEILING_DIRECTORIES: join(cwd, '..') }
  for (const key of Object.keys(env))
    if (key.startsWith('GIT_') && key !== 'GIT_CEILING_DIRECTORIES') delete env[key]
  return execFileSync('git', args, { cwd, env, encoding: 'utf8' })
}

/**
 * A realistically sized workspace repo: a clone of THIS checkout's HEAD, with
 * a bare `origin` beside it so fetch paths have a remote, and a sibling
 * worktree (not imported) the fetch scenario runs `git fetch` in.
 */
export function createCloneFixture(runDir) {
  // "github.com" in the remote's path makes the daemon detect a GitHub repo
  // and run its gh pollers; the fetches themselves stay local.
  const origin = join(runDir, 'remotes', 'github.com', 'meter', 'crowbar-fixture.git')
  const projectDir = join(runDir, 'project')
  const repoDir = join(projectDir, 'crowbar-fixture')
  const sibling = join(runDir, 'sibling')
  const pusher = join(runDir, 'pusher')
  mkdirSync(projectDir, { recursive: true })
  const head = git(REPO_ROOT, 'rev-parse', 'HEAD').trim()
  mkdirSync(origin, { recursive: true })
  git(runDir, 'init', '--quiet', '--bare', '-b', 'main', origin)
  git(REPO_ROOT, 'push', '--quiet', origin, `${head}:refs/heads/main`)
  git(projectDir, 'clone', '--quiet', origin, repoDir)
  // Off main, so the protected main gets its own locked worktree as it does
  // for a user whose checkout sits on a feature branch.
  git(repoDir, 'checkout', '--quiet', '-b', 'meter-home')
  git(repoDir, 'worktree', 'add', '--quiet', '-b', 'meter-sibling', sibling)
  git(runDir, 'clone', '--quiet', origin, pusher)
  for (const dir of [repoDir, pusher]) {
    git(dir, 'config', 'user.name', 'Meter')
    git(dir, 'config', 'user.email', 'meter@crowbar.local')
  }
  const files = git(repoDir, 'ls-files', '-z').split('\0').filter(Boolean).length
  return { origin, projectDir, repoDir, sibling, pusher, files }
}

/** Lands a new commit on origin/main from outside the fixture, so the next fetch has work to do. */
export function advanceOrigin(fx, label) {
  appendFileSync(join(fx.pusher, 'METER.md'), `${label}\n`)
  git(fx.pusher, 'add', 'METER.md')
  git(fx.pusher, 'commit', '--quiet', '-m', `meter: ${label}`)
  git(fx.pusher, 'push', '--quiet', 'origin', 'HEAD:main')
}
