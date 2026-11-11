/** Built once on first use: a formatter resolves the locale when constructed, and
 *  `toLocale*String` builds a fresh one per call — once per row on every mount. */
function memoFormat(options: Intl.DateTimeFormatOptions): (date: Date) => string {
  let formatter: Intl.DateTimeFormat | undefined
  return (date) => (formatter ??= new Intl.DateTimeFormat(undefined, options)).format(date)
}

const dateThisYear = memoFormat({ month: 'short', day: 'numeric' })
const dateOtherYear = memoFormat({ month: 'short', day: 'numeric', year: 'numeric' })
const timeOfDay = memoFormat({ hour: 'numeric', minute: '2-digit' })
const fullTitle = memoFormat({ dateStyle: 'medium', timeStyle: 'short' })

/** "Aug 30, 6:36 PM" — the year only when it isn't the current one. */
function absoluteLabel(then: number): string {
  const date = new Date(then)
  const sameYear = date.getFullYear() === new Date().getFullYear()
  return `${(sameYear ? dateThisYear : dateOtherYear)(date)}, ${timeOfDay(date)}`
}

/** "Aug 30, 6:36 PM": where the hover footer says when a reply was written. */
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
