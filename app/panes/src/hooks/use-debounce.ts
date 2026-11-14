import { useEffect, useState } from 'react'

/** `value`, held back until unchanged for `delayMs`, as a one-element tuple. */
export function useDebounce<T>(value: T, delayMs: number): [T] {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs)
    return () => clearTimeout(timer)
  }, [value, delayMs])
  return [debounced]
}
