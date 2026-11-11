/** One input of an elicitation form, read off the provider's JSON schema. */
export interface FormField {
  key: string
  label: string
  description?: string
  required: boolean
  kind: 'text' | 'number' | 'integer' | 'boolean' | 'choice'
  options?: { value: string; label: string }[]
  initial?: string | boolean
  min?: number
  max?: number
  minLength?: number
  maxLength?: number
}

export type FormValues = Record<string, string | boolean>

type Raw = Record<string, unknown>
const isRaw = (value: unknown): value is Raw =>
  typeof value === 'object' && value !== null && !Array.isArray(value)
const num = (value: unknown) => (typeof value === 'number' ? value : undefined)

function choices(prop: Raw): FormField['options'] {
  if (Array.isArray(prop.enum)) {
    return prop.enum.map((value) => ({ value: String(value), label: String(value) }))
  }
  if (!Array.isArray(prop.oneOf)) return undefined
  const rows = prop.oneOf.filter((row): row is Raw => isRaw(row) && 'const' in row)
  if (rows.length === 0 || rows.length !== prop.oneOf.length) return undefined
  return rows.map((row) => ({
    value: String(row.const),
    label: typeof row.title === 'string' ? row.title : String(row.const),
  }))
}

function parseField(key: string, prop: unknown, required: boolean): FormField | null {
  if (!isRaw(prop)) return null
  const options = choices(prop)
  const type = prop.type
  let kind: FormField['kind']
  if (options && (type === 'string' || type === undefined)) kind = 'choice'
  else if (type === 'string') kind = 'text'
  else if (type === 'number' || type === 'integer' || type === 'boolean') kind = type
  else return null
  const hasDefault = typeof prop.default === 'string' || typeof prop.default === 'boolean'
  return {
    key,
    kind,
    required,
    options,
    label: typeof prop.title === 'string' ? prop.title : key,
    description: typeof prop.description === 'string' ? prop.description : undefined,
    initial: hasDefault ? (prop.default as string | boolean) : undefined,
    min: num(prop.minimum),
    max: num(prop.maximum),
    minLength: num(prop.minLength),
    maxLength: num(prop.maxLength),
  }
}

/** The form a schema describes, or null when any property is one Crowbar cannot draw: a
 *  half-built form would ask for the wrong things, so the terminal answers those. */
export function parseForm(schema: string | undefined): FormField[] | null {
  if (!schema) return null
  let root: unknown
  try {
    root = JSON.parse(schema)
  } catch {
    return null
  }
  if (!isRaw(root) || !isRaw(root.properties)) return null
  const required = new Set(Array.isArray(root.required) ? root.required : [])
  const fields = Object.entries(root.properties).map(([key, prop]) =>
    parseField(key, prop, required.has(key)),
  )
  if (fields.length === 0 || fields.some((field) => field === null)) return null
  return fields as FormField[]
}

export function initialValues(fields: FormField[]): FormValues {
  return Object.fromEntries(
    fields.map((field) => [field.key, field.initial ?? (field.kind === 'boolean' ? false : '')]),
  )
}

function numberError(field: FormField, text: string): string {
  const value = Number(text)
  if (Number.isNaN(value)) return `${field.key} must be a number`
  if (field.kind === 'integer' && !Number.isInteger(value))
    return `${field.key} must be a whole number`
  if (field.min !== undefined && value < field.min)
    return `${field.key} must be at least ${field.min}`
  if (field.max !== undefined && value > field.max)
    return `${field.key} must be at most ${field.max}`
  return ''
}

/** Why a value is not acceptable yet, or '' when it is. */
export function fieldError(field: FormField, value: string | boolean): string {
  if (typeof value === 'boolean') return ''
  const text = value.trim()
  if (text === '') return field.required ? `${field.key} is required` : ''
  if (field.kind === 'number' || field.kind === 'integer') return numberError(field, text)
  if (field.minLength !== undefined && text.length < field.minLength)
    return `${field.key} needs at least ${field.minLength} characters`
  if (field.maxLength !== undefined && text.length > field.maxLength)
    return `${field.key} takes at most ${field.maxLength} characters`
  return ''
}

/** The answer's `content`: typed values, optional blanks left out. */
export function formContent(fields: FormField[], values: FormValues): Record<string, unknown> {
  const content: Record<string, unknown> = {}
  for (const field of fields) {
    const value = values[field.key]
    if (typeof value === 'boolean') content[field.key] = value
    else if (value.trim() !== '')
      content[field.key] =
        field.kind === 'number' || field.kind === 'integer' ? Number(value) : value
  }
  return content
}
