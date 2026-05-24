// Stub: onboarding feature is out of scope for this session.
export type OnboardingMode = 'welcome' | 'update'

export interface OnboardingContext {
  mode: OnboardingMode
  currentVersion?: string
  previousVersion?: string
}
