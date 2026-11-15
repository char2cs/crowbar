const LENGTH_PREFIX_BYTES = 4

/** Encodes a payload with a length prefix so a reader never scans for a delimiter. */
export function encodeFrame(payload: Uint8Array): Uint8Array {
  const out = new Uint8Array(payload.length + LENGTH_PREFIX_BYTES)
  out.set(payload, LENGTH_PREFIX_BYTES)
  return out
}
