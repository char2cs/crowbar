import { encodeFrame } from '@/lib/frame-codec'

it.each([[0, 4], [2, 6]])('prefixes a %i byte payload to %i bytes', (size, total) => {
  expect(encodeFrame(new Uint8Array(size)).length).toBe(total)
})
