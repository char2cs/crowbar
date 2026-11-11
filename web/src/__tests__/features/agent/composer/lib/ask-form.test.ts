import { describe, expect, it } from 'vitest'
import {
  fieldError,
  formContent,
  initialValues,
  parseForm,
} from '@/features/agent/composer/lib/ask-form'

const schema = (properties: object, required: string[] = []) =>
  JSON.stringify({ type: 'object', properties, required })

describe('parseForm', () => {
  it('builds one field per property, in order, with required marked', () => {
    const fields = parseForm(
      schema(
        {
          name: { type: 'string', title: 'Name' },
          region: { type: 'string', enum: ['us', 'eu'] },
          notify: { type: 'boolean', description: 'Tell me' },
          count: { type: 'integer' },
        },
        ['name'],
      ),
    )
    expect(fields?.map((f) => [f.key, f.kind, f.label, f.required])).toEqual([
      ['name', 'text', 'Name', true],
      ['region', 'choice', 'region', false],
      ['notify', 'boolean', 'notify', false],
      ['count', 'integer', 'count', false],
    ])
    expect(fields?.[1].options).toEqual([
      { value: 'us', label: 'us' },
      { value: 'eu', label: 'eu' },
    ])
    expect(fields?.[2].description).toBe('Tell me')
  })

  it('reads titled choices from oneOf', () => {
    const fields = parseForm(
      schema({ tier: { oneOf: [{ const: 'a', title: 'Alpha' }, { const: 'b' }] } }),
    )
    expect(fields?.[0].options).toEqual([
      { value: 'a', label: 'Alpha' },
      { value: 'b', label: 'b' },
    ])
  })

  it.each([
    ['unparseable', '{nope'],
    ['no properties', schema({})],
    ['a nested object', schema({ a: { type: 'object' } })],
    ['an array', schema({ a: { type: 'array', items: { type: 'string' } } })],
    ['a property with no type', schema({ a: {} })],
  ])('is not buildable with %s', (_, raw) => {
    expect(parseForm(raw)).toBeNull()
  })

  it('is not buildable without a schema', () => {
    expect(parseForm(undefined)).toBeNull()
  })
})

describe('validation and content', () => {
  const fields = parseForm(
    schema(
      {
        name: { type: 'string', minLength: 2 },
        n: { type: 'number', minimum: 1, maximum: 5 },
        i: { type: 'integer' },
        on: { type: 'boolean', default: true },
      },
      ['name'],
    ),
  )!

  it('seeds defaults and booleans', () => {
    expect(initialValues(fields)).toEqual({ name: '', n: '', i: '', on: true })
  })

  it('says why a field is wrong, in the field’s own words', () => {
    expect(fieldError(fields[0], '')).toBe('name is required')
    expect(fieldError(fields[0], '', true)).toBe('Required')
    expect(fieldError(fields[0], 'a')).toBe('name needs at least 2 characters')
    expect(fieldError(fields[0], 'ab')).toBe('')
    expect(fieldError(fields[1], 'x')).toBe('n must be a number')
    expect(fieldError(fields[1], '9')).toBe('n must be at most 5')
    expect(fieldError(fields[1], '0')).toBe('n must be at least 1')
    expect(fieldError(fields[2], '1.5')).toBe('i must be a whole number')
    expect(fieldError(fields[1], '')).toBe('')
  })

  it('sends typed values and leaves out empty optional ones', () => {
    expect(formContent(fields, { name: 'ab', n: '3', i: '', on: false })).toEqual({
      name: 'ab',
      n: 3,
      on: false,
    })
  })
})
