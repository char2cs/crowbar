// Stub
export const telemetry = {
  track: (_event: string, _properties?: Record<string, unknown>) => {},
  identify: (_userId: string, _traits?: Record<string, unknown>) => {},
  page: (_name: string) => {},
}
export default telemetry
