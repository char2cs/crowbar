import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'

interface OptimisticOptions<TVariables, TSnapshot> {
  onMutate: (vars: TVariables) => Promise<TSnapshot>
  onSnapshot: (snapshot: TSnapshot) => void
  invalidateKey: QueryKey
}

export function useOptimisticMutation<TData, TVariables, TSnapshot = unknown>(
  mutationFn: (vars: TVariables) => Promise<TData>,
  options: OptimisticOptions<TVariables, TSnapshot>
) {
  const qc = useQueryClient()

  return useMutation<TData, Error, TVariables, TSnapshot>({
    mutationFn,
    onMutate: options.onMutate,
    onError: (_err, _vars, context) => {
      if (context !== undefined) {
        options.onSnapshot(context)
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: options.invalidateKey })
    },
  })
}
