import { useEffect, useState } from 'react'

/**
 * `value`, held back until it has stopped changing for `delayMs`. Returned as a
 * one-element tuple (the `use-debounce` package's shape this replaces).
 */
export function useDebounce<T>(value: T, delayMs: number): [T] {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs)
    return () => clearTimeout(timer)
  }, [value, delayMs])
  return [debounced]
}
