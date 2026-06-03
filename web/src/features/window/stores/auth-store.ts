// Stub: window/auth feature is out of scope for this session.
import { create } from 'zustand'

export interface EnterprisePolicy {
  managedMode?: boolean
  allowByok?: boolean
  [key: string]: unknown
}

export interface SubscriptionInfo {
  plan: string
  status?: "free" | "pro" | "team" | "enterprise"
  enterprise?: { has_access: boolean; policy?: EnterprisePolicy }
  collaboration?: { enabled: boolean; channelNotes?: Array<{ channelId: string; contentMarkdown: string }> }
}

export interface AuthState {
  user: { id: string; email: string; name: string } | null
  isAuthenticated: boolean
  subscription: SubscriptionInfo | null
  isLoading: boolean
  setUser: (user: AuthState["user"]) => void
  setSubscription: (subscription: SubscriptionInfo | null) => void
  setCollaborationSnapshot: (snapshot: unknown) => void
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,
  subscription: null,
  isLoading: false,
  setUser: (user) => set({ user, isAuthenticated: !!user }),
  setSubscription: (subscription) => set({ subscription }),
  setCollaborationSnapshot: (_snapshot) => {},
}))
