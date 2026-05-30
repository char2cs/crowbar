import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useOptimisticMutation } from '@/lib/mutations/optimistic'
import { queryKeys } from '@/lib/queries/keys'

function makeWrapper(qc: QueryClient) {
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

describe('useOptimisticMutation', () => {
  it('calls mutationFn and invalidates on success', async () => {
    const qc = new QueryClient()
    vi.spyOn(qc, 'invalidateQueries')

    const mutationFn = vi.fn().mockResolvedValue({ ok: true })
    const onMutate = vi.fn().mockResolvedValue('snapshot')
    const onSnapshot = vi.fn()

    const { result } = renderHook(
      () => useOptimisticMutation(mutationFn, {
        onMutate,
        onSnapshot,
        invalidateKey: queryKeys.workspaces.all,
      }),
      { wrapper: makeWrapper(qc) }
    )

    result.current.mutate({ name: 'test' })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mutationFn).toHaveBeenCalled()
    expect(mutationFn.mock.calls[0][0]).toEqual({ name: 'test' })
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.workspaces.all,
    })
  })

  it('calls onSnapshot with the snapshot on error', async () => {
    const qc = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    const mutationFn = vi.fn().mockRejectedValue(new Error('fail'))
    const snapshot = { previous: [1, 2, 3] }
    const onMutate = vi.fn().mockResolvedValue(snapshot)
    const onSnapshot = vi.fn()

    const { result } = renderHook(
      () => useOptimisticMutation(mutationFn, {
        onMutate,
        onSnapshot,
        invalidateKey: queryKeys.workspaces.all,
      }),
      { wrapper: makeWrapper(qc) }
    )

    result.current.mutate({})

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(onSnapshot).toHaveBeenCalledWith(snapshot)
  })
})
