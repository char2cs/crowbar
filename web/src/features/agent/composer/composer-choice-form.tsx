import { useId } from 'react'
import { Input } from '@/components/ui/input'
import { Radio, RadioGroup } from '@/components/ui/radio-group'
import { Switch } from '@/components/ui/switch'
import { fieldError, type FormField, type FormValues } from '@/features/agent/composer/lib/ask-form'

/** One field per property of the elicitation's schema. A field says what is wrong with it once it
 *  has been left or a submit was tried, never before the person has typed anything. */
export function ChoiceForm({
  fields,
  values,
  shown,
  busy,
  onChange,
  onLeave,
}: {
  fields: FormField[]
  values: FormValues
  /** Keys whose errors may be drawn. */
  shown: ReadonlySet<string>
  busy: boolean
  onChange: (key: string, value: string | boolean) => void
  onLeave: (key: string) => void
}) {
  const scope = useId()
  return (
    <div className="rq-form" data-testid="agent-choice-form">
      {fields.map((field) => {
        const error = shown.has(field.key) ? fieldError(field, values[field.key]) : ''
        return (
          <div key={field.key} className="rq-field">
            <span className="rq-field-label" id={`${scope}-${field.key}`}>
              {field.label}
              {field.required && <i aria-hidden="true">*</i>}
            </span>
            <div className="rq-field-control">
              <FieldControl
                field={field}
                labelId={`${scope}-${field.key}`}
                value={values[field.key]}
                invalid={error !== ''}
                busy={busy}
                onChange={(value) => onChange(field.key, value)}
                onLeave={() => onLeave(field.key)}
              />
              {error ? (
                <span className="rq-field-error" role="alert">
                  {error}
                </span>
              ) : (
                field.description && <span className="rq-field-help">{field.description}</span>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}

function FieldControl({
  field,
  labelId,
  value,
  invalid,
  busy,
  onChange,
  onLeave,
}: {
  field: FormField
  labelId: string
  value: string | boolean
  invalid: boolean
  busy: boolean
  onChange: (value: string | boolean) => void
  onLeave: () => void
}) {
  if (field.kind === 'boolean') {
    return (
      <Switch
        aria-labelledby={labelId}
        checked={value === true}
        disabled={busy}
        onCheckedChange={onChange}
      />
    )
  }
  if (field.kind === 'choice') {
    return (
      <RadioGroup
        className="rq-choice"
        aria-labelledby={labelId}
        value={String(value)}
        onValueChange={(next) => {
          onChange(String(next))
          onLeave()
        }}
      >
        {field.options?.map((option) => (
          <label key={option.value}>
            <Radio value={option.value} disabled={busy} />
            <span>{option.label}</span>
          </label>
        ))}
      </RadioGroup>
    )
  }
  return (
    <Input
      aria-labelledby={labelId}
      aria-invalid={invalid || undefined}
      inputMode={field.kind === 'text' ? undefined : 'decimal'}
      value={String(value)}
      disabled={busy}
      onChange={(event) => onChange(event.target.value)}
      onBlur={onLeave}
    />
  )
}
