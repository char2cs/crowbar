export const DEFAULT_PORT = 3737

const NOT_AN_ADDRESS = `Enter a host name or IP address, optionally with a port, like homelab.example.com:${DEFAULT_PORT}.`
const NOT_A_PORT = 'The port must be a number from 1 to 65535.'

type Normalized = { endpoint: string } | { error: string }

export function normalizeEndpoint(typed: string): Normalized {
  const bare = typed
    .trim()
    .replace(/^(?:https?|tcp):\/\//i, '')
    .replace(/\/$/, '')
  const parts = /[\s@/?#%\\]/.test(bare) ? null : splitHostPort(bare)
  if (!parts) return { error: NOT_AN_ADDRESS }
  const port = parts.port === undefined ? DEFAULT_PORT : parsePort(parts.port)
  if (port === null) return { error: NOT_A_PORT }
  const host = parts.ipv6 ? ipv6Host(parts.host) : nameOrIpv4Host(parts.host)
  return host ? { endpoint: `${host}:${port}` } : { error: NOT_AN_ADDRESS }
}

interface HostPort {
  host: string
  port?: string
  ipv6: boolean
}

function splitHostPort(text: string): HostPort | null {
  if (text.startsWith('[')) {
    const bracketed = /^\[([^\]]*)\](?::(.*))?$/.exec(text)
    return bracketed ? { host: bracketed[1], port: bracketed[2], ipv6: true } : null
  }
  const [host, port, ...rest] = text.split(':')
  if (rest.length > 0) return { host: text, ipv6: true }
  return host ? { host, port, ipv6: false } : null
}

function parsePort(text: string): number | null {
  const port = /^\d{1,5}$/.test(text) ? Number(text) : 0
  return port >= 1 && port <= 65535 ? port : null
}

function ipv6Host(text: string): string | null {
  if (!/^[\da-f:.]+$/i.test(text)) return null
  try {
    return new URL(`http://[${text}]/`).hostname
  } catch {
    return null
  }
}

const LABEL = /^[a-z\d](?:[a-z\d-]{0,61}[a-z\d])?$/
const OCTET = /^(?:0|[1-9]\d{0,2})$/

function nameOrIpv4Host(text: string): string | null {
  const host = toAscii(text.toLowerCase())
  if (!host) return null
  const labels = host.split('.')
  if (/^\d+$/.test(labels[labels.length - 1])) {
    const ipv4 = labels.length === 4 && labels.every((l) => OCTET.test(l) && Number(l) <= 255)
    return ipv4 ? host : null
  }
  return host.length <= 253 && labels.every((l) => LABEL.test(l)) ? host : null
}

function toAscii(host: string): string | null {
  if (/^[\x21-\x7e]*$/.test(host)) return host
  try {
    return new URL(`http://${host}/`).hostname
  } catch {
    return null
  }
}
