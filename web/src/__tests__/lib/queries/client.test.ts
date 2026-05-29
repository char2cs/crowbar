import { queryClient } from '@/lib/queries/client'

describe('queryClient', () => {
  it('is a QueryClient instance', () => {
    expect(queryClient).toBeDefined()
    expect(typeof queryClient.invalidateQueries).toBe('function')
  })

  it('uses staleTime Infinity as default', () => {
    const defaults = queryClient.getDefaultOptions()
    expect(defaults.queries?.staleTime).toBe(Infinity)
  })

  it('retries once on failure', () => {
    const defaults = queryClient.getDefaultOptions()
    expect(defaults.queries?.retry).toBe(1)
  })
})
