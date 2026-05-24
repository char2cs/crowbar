// Stub
import type { OnboardingMode } from "@/features/onboarding/lib/onboarding-state"

interface OnboardingViewProps {
  bufferId?: string
  context?: {
    mode: OnboardingMode
    currentVersion: string
    previousVersion?: string
  }
  [key: string]: unknown
}
export function OnboardingView(_props: OnboardingViewProps) { return null }
export default OnboardingView
