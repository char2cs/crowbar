// PLATFORM: ios: AppScheme asks the Rust host for a dev URL; the test bundle has no host, so the
// PLATFORM: ios: stub answers "no dev server" and the scheme serves bundled assets.
@_cdecl("crowbar_chrome_dev_url")
func crowbarChromeDevURLStub(_ frontend: UnsafeRawPointer) -> UnsafePointer<CChar>? { nil }
