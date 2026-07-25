#!/usr/bin/env node
/**
 * refresh-tree-sitter-grammars.mjs
 *
 * MAINTENANCE TOOL — not part of dev/build. Regenerates the vendored grammar
 * assets in web/public/tree-sitter/parsers/ from the upstream repositories
 * listed in tree-sitter-grammars.json, then records the exact commit each was
 * built from in that same file.
 *
 * Why the output is committed rather than built on demand: the build must work
 * on a fresh clone with no network and no toolchain. Compiling 40 grammars
 * needs git, a tree-sitter CLI and a wasi-sdk download, none of which belong in
 * a normal `bun run build`. So we build here, deliberately, and commit results.
 *
 * Why we build rather than pull prebuilt wasm from npm: every binary in
 * `tree-sitter-wasms` uses the legacy Emscripten `dylink` section, which the
 * current web-tree-sitter runtime cannot instantiate — all 36 fail to load.
 * Building with a current tree-sitter CLI emits the modern `dylink.0` format.
 *
 * The critical property this gives us is that each grammar and its
 * highlights.scm come from the SAME upstream commit. A query naming a node type
 * its grammar does not define throws at Query construction and silently
 * disables highlighting for that language — the exact failure mode that had
 * gone unnoticed for 12 of the 40 languages.
 *
 * Usage:
 *   node scripts/refresh-tree-sitter-grammars.mjs            # all grammars
 *   node scripts/refresh-tree-sitter-grammars.mjs rust lua   # only these
 *
 * Always follow with `node scripts/verify-tree-sitter.mjs`.
 */

import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { tmpdir } from 'node:os'

const __dirname = dirname(fileURLToPath(import.meta.url))
const WEB_ROOT = resolve(__dirname, '..')
const MANIFEST_PATH = join(__dirname, 'tree-sitter-grammars.json')
const PARSERS_DIR = join(WEB_ROOT, 'public', 'tree-sitter', 'parsers')
const WORK = join(tmpdir(), 'crowbar-tree-sitter-build')

// Candidate highlight-query locations, tried in order. Upstreams disagree on
// layout: single-grammar repos use queries/highlights.scm, while multi-grammar
// repos put them under queries/<grammar>/ relative to the REPO root even though
// the grammar itself lives in a subdirectory. `{name}` is the parser folder name.
const DEFAULT_QUERY_PATHS = [
  'queries/highlights.scm',
  'queries/{name}/highlights.scm',
  'queries/highlights-nvim.scm',
  '../queries/highlights.scm',
  '../queries/{name}/highlights.scm',
  '../../queries/highlights.scm',
  '../../queries/{name}/highlights.scm',
]

/**
 * Strip `(#set! ...)` directives from a highlight query.
 *
 * Editor-specific query sets (notably nvim-treesitter's) attach metadata with
 * three-argument forms such as `(#set! @capture "priority" 105)`. web-tree-sitter
 * only accepts one or two arguments and throws `Wrong number of arguments to
 * '#set!' predicate` for the rest, which fails the WHOLE query and silently
 * disables highlighting for that language.
 *
 * These directives are pure metadata — priority hints and injection settings.
 * Our tokenizer highlights from capture names alone (see tokenizer.ts, which
 * reads query.captures() and never inspects predicate metadata), so dropping
 * them removes nothing we render. Removal is s-expression aware so the
 * surrounding pattern stays intact.
 */
export function stripSetDirectives(source) {
  let out = ''
  for (let i = 0; i < source.length; i++) {
    if (source.startsWith('(#set!', i)) {
      // Skip the balanced s-expression, ignoring parens inside strings.
      let depth = 0
      let inString = false
      let j = i
      for (; j < source.length; j++) {
        const ch = source[j]
        if (inString) {
          if (ch === '\\') j++
          else if (ch === '"') inString = false
          continue
        }
        if (ch === '"') inString = true
        else if (ch === '(') depth++
        else if (ch === ')') {
          depth--
          if (depth === 0) break
        }
      }
      i = j
      continue
    }
    out += source[i]
  }
  // Collapse blank space left where directives were removed.
  return out.replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n')
}

function run(cmd, args, cwd) {
  return execFileSync(cmd, args, { cwd, stdio: ['ignore', 'pipe', 'pipe'] }).toString()
}

function treeSitterCli() {
  // Prefer a locally installed CLI; otherwise fall back to npx with a version
  // matched to the web-tree-sitter runtime generation.
  const local = join(WEB_ROOT, 'node_modules', '.bin', 'tree-sitter')
  if (existsSync(local)) return { cmd: local, args: [] }
  return { cmd: 'npx', args: ['--yes', 'tree-sitter-cli@0.26.11'] }
}

async function main() {
  const manifest = JSON.parse(readFileSync(MANIFEST_PATH, 'utf8'))
  const only = process.argv.slice(2)
  const names = Object.keys(manifest.grammars).filter((n) => only.length === 0 || only.includes(n))

  mkdirSync(WORK, { recursive: true })
  const cli = treeSitterCli()

  const results = []
  for (const name of names) {
    const spec = manifest.grammars[name]
    const repoDir = join(WORK, spec.repo.replace('/', '__'))
    const result = { name, status: 'ok', note: '' }
    results.push(result)

    try {
      if (!existsSync(repoDir)) {
        run('git', ['clone', '--depth', '1', `https://github.com/${spec.repo}.git`, repoDir])
      }
      const sha = run('git', ['rev-parse', 'HEAD'], repoDir).trim()
      const date = run('git', ['log', '-1', '--format=%cs'], repoDir).trim()

      const grammarDir = spec.subdir ? join(repoDir, spec.subdir) : repoDir

      // Some grammars ship no generated parser.c; generate it first.
      if (!existsSync(join(grammarDir, 'src', 'parser.c'))) {
        try {
          run(cli.cmd, [...cli.args, 'generate'], grammarDir)
        } catch (err) {
          throw new Error(`generate failed: ${String(err.stderr || err).slice(0, 200)}`, {
            cause: err,
          })
        }
      }

      const outDir = join(PARSERS_DIR, name)
      mkdirSync(outDir, { recursive: true })
      const wasmOut = join(outDir, 'parser.wasm')
      try {
        run(cli.cmd, [...cli.args, 'build', '--wasm', '-o', wasmOut, grammarDir])
      } catch (err) {
        throw new Error(`build failed: ${String(err.stderr || err).slice(0, 200)}`, {
          cause: err,
        })
      }

      // Highlight query: prefer one from this same commit so grammar and query
      // are guaranteed consistent.
      let querySrc = null
      const candidates = spec.queries || DEFAULT_QUERY_PATHS
      for (const template of candidates) {
        const rel = template.replace('{name}', name)
        const p = resolve(grammarDir, rel)
        if (existsSync(p)) {
          querySrc = p
          break
        }
      }

      // An explicit external query in the manifest wins over the repo's own.
      const useExternal = Boolean(spec.queryUrls || spec.queryUrl)
      if (querySrc && !useExternal) {
        // Several grammars extend another grammar and ship only the delta —
        // tree-sitter-typescript's query is 35 lines because it is meant to sit
        // on top of tree-sitter-javascript's 204. Taking the delta alone costs
        // most of the highlighting (tsx fell from 154 patterns to 7). `compose`
        // names the base query layers to prepend. Order matters: the tokenizer
        // keeps the LAST capture for a given range, so bases come first and the
        // grammar's own, more specific layer wins.
        const parts = []
        for (const base of spec.compose || []) {
          const baseRepoDir = join(WORK, base.repo.replace('/', '__'))
          if (!existsSync(baseRepoDir)) {
            run('git', [
              'clone',
              '--depth',
              '1',
              `https://github.com/${base.repo}.git`,
              baseRepoDir,
            ])
          }
          const basePath = join(baseRepoDir, base.path)
          if (!existsSync(basePath))
            throw new Error(`compose source missing: ${base.repo}/${base.path}`)
          const baseSha = run('git', ['rev-parse', 'HEAD'], baseRepoDir).trim()
          parts.push(
            `; --- from ${base.repo}@${baseSha.slice(0, 10)} ${base.path} ---\n${stripSetDirectives(readFileSync(basePath, 'utf8'))}`,
          )
          base.commit = baseSha
        }
        parts.push(
          `; --- from ${spec.repo}@${sha.slice(0, 10)} ${querySrc.replace(`${repoDir}/`, '')} ---\n${stripSetDirectives(readFileSync(querySrc, 'utf8'))}`,
        )
        writeFileSync(join(outDir, 'highlights.scm'), parts.join('\n'))
        const composed = spec.compose?.length ? ` +${spec.compose.length} base layer(s)` : ''
        result.note = `${sha.slice(0, 10)} ${date} query=${querySrc.replace(`${repoDir}/`, '')}${composed}`
      } else if (spec.queryUrl || spec.queryUrls) {
        // Some upstreams ship no query, and some ship one so thin it loses most
        // highlighting (tree-sitter-typescript's own query yields 41 patterns
        // against tsx where the editor-oriented set yields 156). In those cases
        // the query comes from an external, separately-licensed source pinned to
        // a commit; `queryLicense` records that source's license for NOTICE.md.
        const urls = spec.queryUrls || [spec.queryUrl]
        const parts = []
        for (const url of urls) {
          const res = await fetch(url)
          if (!res.ok) throw new Error(`query fetch ${res.status} ${url}`)
          parts.push(`; --- from ${url} ---\n${stripSetDirectives(await res.text())}`)
        }
        writeFileSync(join(outDir, 'highlights.scm'), parts.join('\n'))
        result.note = `${sha.slice(0, 10)} ${date} query=<external x${urls.length}>`
      } else {
        result.status = 'no-query'
        result.note = `${sha.slice(0, 10)} ${date} NO QUERY FOUND`
      }

      spec.commit = sha
      spec.commitDate = date
    } catch (err) {
      result.status = 'FAIL'
      result.note = String(err.message || err)
        .replace(/\s+/g, ' ')
        .slice(0, 160)
    }

    console.log(
      `${result.status === 'ok' ? ' ok ' : result.status.padEnd(4)} ${name.padEnd(12)} ${result.note}`,
    )
  }

  writeFileSync(MANIFEST_PATH, `${JSON.stringify(manifest, null, 2)}\n`)

  const bad = results.filter((r) => r.status !== 'ok')
  console.log(`\n${results.length - bad.length}/${results.length} grammars refreshed`)
  if (bad.length) {
    console.log(`needs attention: ${bad.map((b) => b.name).join(', ')}`)
  }
  console.log('\nNow run: node scripts/verify-tree-sitter.mjs')
}

// Only rebuild when run directly. This module also exports stripSetDirectives
// for tests, and importing it must not trigger a 40-grammar rebuild.
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main()
}
