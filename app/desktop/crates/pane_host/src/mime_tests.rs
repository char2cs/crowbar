use super::*;

#[test]
fn maps_common_extensions() {
    let cases = [
        ("index.html", "text/html; charset=utf-8"),
        ("a/b.JS", "text/javascript; charset=utf-8"),
        ("x.woff2", "font/woff2"),
        ("x.wasm", "application/wasm"),
        ("noextension", "application/octet-stream"),
        ("weird.xyz", "application/octet-stream"),
    ];
    for (path, mime) in cases {
        assert_eq!(mime_for(path), mime, "{path}");
    }
}
