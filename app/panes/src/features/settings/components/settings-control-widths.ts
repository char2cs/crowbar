// Shared control-width tokens for settings rows. Kept in a plain module (not the
// settings-section component file) so those component files stay Fast-Refresh-safe.
export const SETTINGS_CONTROL_WIDTHS = {
  compact: 'w-28 max-w-full',
  default: 'w-36 max-w-full',
  wide: 'w-44 max-w-full',
  xwide: 'w-56 max-w-full',
  number: 'w-28 max-w-full',
  numberCompact: 'w-24 max-w-full',
  text: 'w-48 max-w-full',
  textWide: 'w-56 max-w-full',
} as const
