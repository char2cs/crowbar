/** The pane SDK major version every frame carries as `sdk`. */
export const SDK_MAJOR = 1

/** The version of the `settings` frame's vocabulary this SDK reads; a frame of another version is ignored. */
export const SETTINGS_FRAME_VERSION = 1

/** The `<major>.<minor>` version a page announces in `pane.register`. */
export const SDK_VERSION = '1.0'

const BYTES_PER_KIB = 1024
const STATE_LIMIT_KIB = 64

/** Largest serialized pane state the shell accepts, in bytes. */
export const MAX_STATE_BYTES = STATE_LIMIT_KIB * BYTES_PER_KIB

/** Longest tab title the shell accepts, in bytes. */
export const MAX_TITLE_BYTES = 256
