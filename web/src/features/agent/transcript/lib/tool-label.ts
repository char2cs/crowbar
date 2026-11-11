import type { ToolCallBody } from '@/features/agent/api/chat-rows'
import type { ToolItem } from '@/features/agent/stores/lib/transcript-items'

/** Live, done, and the plain verb for a call that never finished. */
const VERBS: Partial<Record<ToolItem['family'], [string, string, string]>> = {
  read: ['Reading', 'Read', 'Read'],
  edit: ['Editing', 'Edited', 'Edit'],
  execute: ['Running', 'Ran', 'Run'],
  search: ['Searching for', 'Searched', 'Search'],
  fetch: ['Fetching', 'Fetched', 'Fetch'],
  subagent: ['Running subagent', 'Ran subagent', 'Subagent'],
}

const CREATE: [string, string, string] = ['Creating', 'Created', 'Create']

/** An edit writes a new file when its diff has no old side, or its one hunk starts
 *  from nothing (how claude's Write reports a file that was not there). */
const writesNewFile = (diff = '') => {
  if (diff.startsWith('--- /dev/null\n')) return true
  const hunks = diff.match(/^@@ .*$/gm) ?? []
  return hunks.length === 1 && hunks[0]!.startsWith('@@ -0,0 ')
}

export interface ToolLabel {
  verb: string
  subject?: { full: string; display: string }
}

function humanizeToolName(name: string): string {
  const words = name
    .replace(/^mcp__.+?__/, '')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .trim()
    .toLowerCase()
  return words ? `${words[0]?.toUpperCase()}${words.slice(1)}` : ''
}

const WORDS = /"([^"]*)"|'([^']*)'|(\S+)/g
const SHELLS = new Set(['bash', 'sh', 'zsh', 'dash'])
const basename = (word: string) => word.split('/').pop() ?? word

/** A command as a reader names it: wrapping shells, sudo and env assignments
 *  peeled off, the program by its basename, first line only. */
export function commandProgramName(cmd: string): string {
  const line = cmd.trim().split('\n')[0] ?? ''
  const words = [...line.matchAll(WORDS)].map((m) => m[1] ?? m[2] ?? m[3] ?? '')
  let i = 0
  while (i < words.length) {
    const word = basename(words[i]!)
    if (word === 'sudo' || word === 'env') {
      i++
      while (i < words.length && (words[i]!.startsWith('-') || words[i]!.includes('='))) i++
    } else if (/^[A-Za-z_][A-Za-z0-9_]*=/.test(words[i]!)) {
      i++
    } else if (SHELLS.has(word) && /^-\w*c\w*$/.test(words[i + 1] ?? '')) {
      return commandProgramName(words[i + 2] ?? '')
    } else break
  }
  const rest = words.slice(i)
  if (rest.length === 0) return ''
  return [basename(rest[0]!), ...rest.slice(1)].join(' ')
}

function hostOf(target: string): { host: string; more: boolean } | undefined {
  try {
    const url = new URL(target)
    return { host: url.host, more: url.pathname.length > 1 || url.search !== '' }
  } catch {
    return undefined // not a URL: the target is shown as it came
  }
}

function subjectOf(item: ToolItem, done: boolean): ToolLabel['subject'] {
  const { body, family } = item
  const location = body.locations?.[0]
  if (location && (family === 'read' || family === 'edit' || family === 'search')) {
    const line = location.line ? ` L${location.line}` : ''
    const more = (body.locations?.length ?? 0) > 1 ? ` +${body.locations!.length - 1}` : ''
    return {
      full: `${location.path}${location.line ? `:${location.line}` : ''}${more}`,
      display: `${location.path}${line}${more}`,
    }
  }
  const target = body.target?.trim() ?? ''
  if (family === 'execute') {
    const shown = body.title || commandProgramName(target) || target
    return shown ? { full: target || shown, display: shown } : undefined
  }
  if (family === 'subagent' && body.title) return { full: body.title, display: body.title }
  if (!target) return undefined
  if (family === 'search') return { full: target, display: `"${target}"` }
  if (family === 'fetch') {
    const url = hostOf(target)
    if (url) return { full: target, display: done && url.more ? `${url.host}/…` : url.host }
  }
  // The daemon names a file inside the workspace from there; an absolute path lies outside it.
  return { full: target, display: target }
}

/** The tool an MCP call named. */
const mcpTool = (body: ToolCallBody) => body.mcpTool || humanizeToolName(body.name ?? '')

/** A call's verb and subject: present while it runs, past once done. */
export function toolLabel(item: ToolItem): ToolLabel {
  const { body, family, status } = item
  const done = status === 'ok' || status === 'error'
  if (family === 'mcp') {
    const tool = mcpTool(body)
    return { verb: body.mcpServer || 'MCP', subject: { full: tool, display: tool } }
  }
  const verbs = family === 'edit' && writesNewFile(body.diff) ? CREATE : VERBS[family]
  const verb = verbs
    ? verbs[status === 'running' ? 0 : done ? 1 : 2]
    : humanizeToolName(body.name ?? '') || 'Tool'
  return { verb, subject: subjectOf(item, done) }
}

const asText = ({ verb, subject }: ToolLabel, family: ToolItem['family']) =>
  family === 'mcp'
    ? `${verb} · ${subject?.display ?? ''}`
    : [verb, subject?.display].filter(Boolean).join(' ')

export const liveLabel = (item: ToolItem) =>
  asText(toolLabel({ ...item, status: 'running' }), item.family)

/** The label as the call's own status reads it. */
export const labelText = (item: ToolItem) => asText(toolLabel(item), item.family)

export function diffStat(diff: string): { added: number; removed: number } {
  let added = 0
  let removed = 0
  for (const line of diff.split('\n')) {
    if (line.startsWith('+') && !line.startsWith('+++')) added++
    else if (line.startsWith('-') && !line.startsWith('---')) removed++
  }
  return { added, removed }
}

/** The newest `limit` characters of live text on one line, emphasis runs
 *  stripped: the newest words say what is happening now. */
export function tailOf(text: string, limit: number): string {
  const flat = text
    .replace(/\*{1,3}/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  if (flat.length <= limit) return flat
  return `…${flat.slice(flat.length - limit)}`
}

/** A duration read at a glance; sub-second work in ms, since "0s" reads as nothing. */
export function formatDuration(ms: number | undefined): string {
  if (!ms || ms < 0) return ''
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  const minutes = Math.floor(ms / 60_000)
  const seconds = Math.round((ms % 60_000) / 1000)
  return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
}
