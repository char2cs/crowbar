// Stub: github feature is out of scope for this session.
import { create } from 'zustand'

interface PRDetails {
  number: number
  changedFiles: number
  commits: unknown[]
  author: { login: string }
  body: string | null
}

export interface GitHubState {
  selectedPRDetails: PRDetails | null
  selectedPRComments: unknown[]
  actions: {
    openPR: (number: number) => void
    closePR: () => void
    fetchPRDetails: (number: number) => Promise<void>
  }
}

export const useGitHubStore = create<GitHubState>(() => ({
  selectedPRDetails: null,
  selectedPRComments: [],
  actions: {
    openPR: () => {},
    closePR: () => {},
    fetchPRDetails: async () => {},
  },
}))
