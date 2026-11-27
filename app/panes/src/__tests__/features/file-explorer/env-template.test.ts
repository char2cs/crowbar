import { describe, expect, it } from 'vitest'
import {
  buildEnvTemplateContent,
  isEnvFileName,
} from '@/features/file-explorer/file-explorer/lib/env-template'

describe('env-template helpers', () => {
  it('detects env file names', () => {
    expect(isEnvFileName('.env')).toBe(true)
    expect(isEnvFileName('.env.local')).toBe(true)
    expect(isEnvFileName('.env.production.local')).toBe(true)
    expect(isEnvFileName('env.local')).toBe(false)
    expect(isEnvFileName('config.env')).toBe(false)
  })

  it('keeps keys and comments while blanking values', () => {
    const source = [
      '# API',
      'API_KEY=secret',
      'export DATABASE_URL=postgres://user:pass@localhost/db # local database',
      'QUOTED="value # not comment"',
      'HASH_VALUE=abc#123',
      '',
    ].join('\n')

    expect(buildEnvTemplateContent(source)).toBe(
      [
        '# API',
        'API_KEY=',
        'export DATABASE_URL= # local database',
        'QUOTED=',
        'HASH_VALUE=',
        '',
      ].join('\n'),
    )
  })
})
