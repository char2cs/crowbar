/** The port a host's daemon listens on unless its address names another. */
export const DEFAULT_PORT = 3737

const NOT_AN_ADDRESS = `Enter a host name or IP address, optionally with a port, like homelab.example.com:${DEFAULT_PORT}.`
const NOT_A_PORT = 'The port must be a number from 1 to 65535.'

type Normalized = { endpoint: string } | { error: string }

/** The one `host:port` an address is probed, paired, saved and compared as, however it was
 *  typed; or, in plain words, why it is none. The desktop's `parse_endpoint` takes only this
 *  form, so nothing it dials was ever guessed at there. */
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

// More than one colon without brackets is an IPv6 address: any port in it would be ambiguous.
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

// The URL parser writes an IPv6 address the one way it compares: lowercase, zeros compressed.
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

// A numeric last label is an IPv4 address or nothing: resolvers read `1.2.3` as one too.
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

// A non-ASCII name is looked up as its punycode, which the URL parser's IDNA mapping writes.
function toAscii(host: string): string | null {
  if (/^[\x21-\x7e]*$/.test(host)) return host
  try {
    return new URL(`http://${host}/`).hostname
  } catch {
    return null
  }
}
