// Stub
import { create } from "zustand"
export const useOnboardingStore = create(() => ({
  isComplete: true,
  currentStep: null as string | null,
  openPreview: () => {},
  open: () => {},
}))
