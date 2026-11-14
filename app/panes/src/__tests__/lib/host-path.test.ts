import { describe, expect, it } from 'vitest'
import { qualify, splitHostPath } from '@/lib/host-path'

describe('qualify', () => {
  it('leaves a local path byte-identical', () => {
    expect(qualify('local', '/v0/projects/p1/repos?x=1')).toBe('/v0/projects/p1/repos?x=1')
  })

  it('prefixes a remote host, keeping the query', () => {
    expect(qualify('host-1', '/v0/ws/git?wsId=w1')).toBe('crowbar://host-1/v0/ws/git?wsId=w1')
    expect(qualify('0f3c9a2e-7b1d-4e6a-9c55-2d8f1b0a6e43', '/v0/projects')).toBe(
      'crowbar://0f3c9a2e-7b1d-4e6a-9c55-2d8f1b0a6e43/v0/projects',
    )
  })

  it.each(['', 'localhost', 'Host-1', 'host@1', 'host:1', 'host/1', 'host.1', 'host 1'])(
    'rejects the host id %j that the desktop proxy refuses',
    (hostId) => {
      expect(() => qualify(hostId, '/v0/projects')).toThrow(/invalid host id/)
    },
  )

  it('rejects a remote path that does not start with a slash', () => {
    expect(() => qualify('host-1', 'v0/projects')).toThrow(/must start with "\/"/)
  })

  it('rejects a remote path that starts with a second slash', () => {
    expect(() => qualify('host-1', '//evil.example/v0/x')).toThrow(/must not start with "\/\/"/)
  })

  it('leaves unqualified local paths alone but refuses to carry another host through local', () => {
    expect(qualify('local', '')).toBe('')
    expect(qualify('local', '//x')).toBe('//x')
    for (const path of [
      'crowbar://host-1/v0/x',
      'crowbar://localhost/v0/x',
      'CROWBAR://host-1/v0/x',
      ' crowbar://host-1/v0/x',
    ]) {
      expect(() => qualify('local', path)).toThrow(/must not be qualified/)
    }
  })
})

describe('splitHostPath', () => {
  it('treats a bare path as local and returns it untouched', () => {
    expect(splitHostPath('/v0/projects/p1')).toEqual({ hostId: 'local', path: '/v0/projects/p1' })
    expect(splitHostPath('http://127.0.0.1:3737/v0/x')).toEqual({
      hostId: 'local',
      path: 'http://127.0.0.1:3737/v0/x',
    })
  })

  it('splits a qualified path into its host and the path with its query', () => {
    expect(splitHostPath('crowbar://host-1/v0/ws/git?wsId=w1')).toEqual({
      hostId: 'host-1',
      path: '/v0/ws/git?wsId=w1',
    })
  })

  it('is the inverse of qualify', () => {
    for (const [hostId, path] of [
      ['local', '/v0/projects'],
      ['host-1', '/v0/projects/p1/repos/r1/workspaces/w1'],
      ['abc123', '/v0/ws/terminal?session=s%201'],
    ]) {
      expect(splitHostPath(qualify(hostId, path))).toEqual({ hostId, path })
    }
  })

  it('reads the desktop local base crowbar://localhost as local, never as a remote host', () => {
    expect(splitHostPath('crowbar://localhost/v0/projects')).toEqual({
      hostId: 'local',
      path: '/v0/projects',
    })
    expect(splitHostPath('crowbar://localhost')).toEqual({ hostId: 'local', path: '' })
  })

  it.each([
    ['crowbar://Host_1/v0/x', /invalid host id "Host_1"/],
    ['crowbar://local/v0/x', /invalid host id "local"/],
    ['crowbar:///v0/x', /invalid host id ""/],
    ['crowbar://host-1', /must start with "\/"/],
    ['crowbar://host-1//evil.example/x', /must not start with "\/\/"/],
  ])('throws on the malformed qualified path %j instead of falling back to local', (input, why) => {
    expect(() => splitHostPath(input)).toThrow(why)
  })

  it.each([
    'CROWBAR://host-1/v0/x',
    'Crowbar://host-1/v0/x',
    ' crowbar://host-1/v0/x',
    '\ncrowbar://host-1/v0/x',
  ])('throws on the wrongly spelled scheme %j instead of passing it through as local', (input) => {
    expect(() => splitHostPath(input)).toThrow(/scheme must be exactly "crowbar:\/\/"/)
  })

  // Intentional divergences from the desktop proxy (see host-path.ts).
  it('is stricter than the proxy on a port or empty authority, and reads Monaco URIs as host ids', () => {
    expect(() => splitHostPath('crowbar://localhost:3737/v0/x')).toThrow(
      /invalid host id "localhost:3737"/,
    )
    expect(() => splitHostPath('crowbar:///')).toThrow(/invalid host id ""/)
    expect(splitHostPath('crowbar://editor/models/1')).toEqual({
      hostId: 'editor',
      path: '/models/1',
    })
  })
})
