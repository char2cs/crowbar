import { describe, expect, it } from 'vitest'
import { normalizeEndpoint } from '@/features/hosts/lib/endpoint'

const INVALID =
  'Enter a host name or IP address, optionally with a port, like homelab.example.com:3737.'
const BAD_PORT = 'The port must be a number from 1 to 65535.'

describe('normalizeEndpoint', () => {
  it.each([
    ['homelab.example.com', 'homelab.example.com:3737'],
    ['  HomeLab.Example.com  ', 'homelab.example.com:3737'],
    ['homelab.example.com:4000', 'homelab.example.com:4000'],
    ['homelab', 'homelab:3737'],
    ['my-host-2.lan', 'my-host-2.lan:3737'],
    ['xn--bcher-kva.example', 'xn--bcher-kva.example:3737'],
    // A scheme and a trailing slash, as an address copied from a browser carries them.
    ['https://Lab.Example.com/', 'lab.example.com:3737'],
    ['http://lab.example.com:8080', 'lab.example.com:8080'],
    ['HTTPS://lab.example.com:8080/', 'lab.example.com:8080'],
    ['tcp://lab.example.com', 'lab.example.com:3737'],
    ['lab.example.com/', 'lab.example.com:3737'],
    ['1.2.3.4', '1.2.3.4:3737'],
    ['1.2.3.4:3737', '1.2.3.4:3737'],
    ['0.0.0.0:1', '0.0.0.0:1'],
    ['255.255.255.255:65535', '255.255.255.255:65535'],
    ['127.0.0.1:38737', '127.0.0.1:38737'],
    // A bare IPv6 address is all colons, so it takes the default port inside brackets.
    ['::1', '[::1]:3737'],
    ['FD00::2', '[fd00::2]:3737'],
    ['[::1]', '[::1]:3737'],
    ['[::1]:4000', '[::1]:4000'],
    ['[FD00:0:0:0:0:0:0:2]:4000', '[fd00::2]:4000'],
    ['https://[::1]:4000/', '[::1]:4000'],
    // A non-ASCII name dials as the punycode its resolver looks up.
    ['Bücher.example', 'xn--bcher-kva.example:3737'],
    [`${'a'.repeat(63)}.com`, `${'a'.repeat(63)}.com:3737`],
    [`${'a.'.repeat(126)}a`, `${'a.'.repeat(126)}a:3737`],
  ])('%s is %s', (typed, endpoint) => {
    expect(normalizeEndpoint(typed)).toEqual({ endpoint })
  })

  it.each([
    ['nothing', ''],
    ['only spaces', '   '],
    ['a space inside', 'home lab.example.com'],
    ['a user', 'me@homelab.example.com'],
    ['a path', 'homelab.example.com/v0'],
    ['a path after the port', 'homelab.example.com:3737/v0'],
    ['a query', 'homelab.example.com/?q=1'],
    ['a bare query', 'homelab.example.com?q=1'],
    ['a fragment', 'homelab.example.com#top'],
    ['another scheme', 'ftp://homelab.example.com'],
    ['a leading hyphen', '-homelab.example.com'],
    ['a trailing hyphen', 'homelab-.example.com'],
    ['an underscore', 'home_lab.example.com'],
    ['an empty label', 'homelab..example.com'],
    ['a trailing dot', 'homelab.example.com.'],
    ['a label over 63 characters', `${'a'.repeat(64)}.com`],
    ['a name over 253 characters', `${'a.'.repeat(126)}ab`],
    ['an octet over 255', '1.2.3.256'],
    ['three octets', '1.2.3'],
    ['an octet with a leading zero', '01.2.3.4'],
    ['a numeric last label', 'host.123'],
    ['an unclosed bracket', '[::1'],
    ['a bracketed name', '[homelab]:3737'],
    ['text after the bracket', '[::1]x'],
    ['an IPv6 zone', 'fe80::1%en0'],
    ['a broken IPv6 address', '1:2:3:4:5:6:7:8:9'],
  ])('refuses %s', (_, typed) => {
    expect(normalizeEndpoint(typed)).toEqual({ error: INVALID })
  })

  it.each([
    ['an empty port', 'homelab.example.com:'],
    ['a named port', 'homelab.example.com:http'],
    ['port 0', 'homelab.example.com:0'],
    ['a port past 65535', 'homelab.example.com:65536'],
    ['a negative port', 'homelab.example.com:-1'],
    ['a signed port', 'homelab.example.com:+80'],
    ['a bracketed address with port 0', '[::1]:0'],
  ])('refuses %s', (_, typed) => {
    expect(normalizeEndpoint(typed)).toEqual({ error: BAD_PORT })
  })
})
