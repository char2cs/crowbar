export const platform = { mobile: false }

export const platformModule = {
  IS_MAC: true,
  get IS_MOBILE() {
    return platform.mobile
  },
  APPLE_KEYBOARD: true,
}
