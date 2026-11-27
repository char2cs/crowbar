function memoFormat(options: Intl.DateTimeFormatOptions): (date: Date) => string {
  let formatter: Intl.DateTimeFormat | undefined
  return (date) => (formatter ??= new Intl.DateTimeFormat(undefined, options)).format(date)
}

const dateThisYear = memoFormat({ month: 'short', day: 'numeric' })
const dateOtherYear = memoFormat({ month: 'short', day: 'numeric', year: 'numeric' })
const timeOfDay = memoFormat({ hour: 'numeric', minute: '2-digit' })
const fullTitle = memoFormat({ dateStyle: 'medium', timeStyle: 'short' })

function absoluteLabel(then: number): string {
  const date = new Date(then)
  const sameYear = date.getFullYear() === new Date().getFullYear()
  return `${(sameYear ? dateThisYear : dateOtherYear)(date)}, ${timeOfDay(date)}`
}

export function turnTimestampLabel(at: string): string {
  const then = Date.parse(at)
  if (Number.isNaN(then)) return ''
  return absoluteLabel(then)
}

export function turnTimeTitle(at: string): string {
  const then = Date.parse(at)
  if (Number.isNaN(then)) return ''
  return fullTitle(new Date(then))
}
