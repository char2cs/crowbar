pub(crate) fn mime_for(path: &str) -> &'static str {
    let extension = path.rsplit_once('.').map_or("", |(_, extension)| extension);
    match extension.to_ascii_lowercase().as_str() {
        "html" | "htm" => "text/html; charset=utf-8",
        "js" | "mjs" => "text/javascript; charset=utf-8",
        "css" => "text/css; charset=utf-8",
        "json" | "map" => "application/json",
        "svg" => "image/svg+xml",
        "png" => "image/png",
        "jpg" | "jpeg" => "image/jpeg",
        "gif" => "image/gif",
        "webp" => "image/webp",
        "ico" => "image/x-icon",
        "woff2" => "font/woff2",
        "woff" => "font/woff",
        "ttf" => "font/ttf",
        "wasm" => "application/wasm",
        "txt" => "text/plain; charset=utf-8",
        _ => "application/octet-stream",
    }
}

#[cfg(test)]
mod tests {
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
}
