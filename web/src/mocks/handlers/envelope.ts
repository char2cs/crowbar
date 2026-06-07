import { HttpResponse } from 'msw'

/** Wrap a payload in the backend's success envelope: { success, error: null, data }. */
export function ok<T>(data: T, status = 200) {
  return HttpResponse.json({ success: true, error: null, data }, { status })
}

/** Build the backend's failure envelope: { success:false, error, data:null }. */
export function fail(error: string, status = 500) {
  return HttpResponse.json({ success: false, error, data: null }, { status })
}
