// desktop/src-tauri/src/browser_proxy.rs

use tauri::{Runtime, UriSchemeContext, UriSchemeResponder};
use tauri::http;

// ── URL helpers ───────────────────────────────────────────────────────────────

/// "crowbar-browser://proxy/https/example.com/path?q=1" → "https://example.com/path?q=1"
pub fn parse_proxy_url(url: &str) -> Option<String> {
    let without_prefix = url.strip_prefix("crowbar-browser://proxy/")?;
    let (protocol, rest) = without_prefix.split_once('/')?;
    match protocol {
        "https" | "http" => Some(format!("{}://{}", protocol, rest)),
        _ => None,
    }
}

/// "https://example.com/path" → "crowbar-browser://proxy/https/example.com/path"
pub fn to_proxy_url(url: &str) -> Option<String> {
    if let Some(rest) = url.strip_prefix("https://") {
        return Some(format!("crowbar-browser://proxy/https/{}", rest));
    }
    if let Some(rest) = url.strip_prefix("http://") {
        return Some(format!("crowbar-browser://proxy/http/{}", rest));
    }
    None
}

// ── Response transformers ─────────────────────────────────────────────────────

fn strip_csp_frame_ancestors(csp: &str) -> String {
    csp.split(';')
        .map(str::trim)
        .filter(|d| !d.to_ascii_lowercase().starts_with("frame-ancestors"))
        .collect::<Vec<_>>()
        .join("; ")
}

fn rewrite_links(html: &str) -> String {
    html.replace("href=\"https://", "href=\"crowbar-browser://proxy/https/")
        .replace("href=\"http://",  "href=\"crowbar-browser://proxy/http/")
        .replace("href='https://",  "href='crowbar-browser://proxy/https/")
        .replace("href='http://",   "href='crowbar-browser://proxy/http/")
        .replace("action=\"https://", "action=\"crowbar-browser://proxy/https/")
        .replace("action=\"http://",  "action=\"crowbar-browser://proxy/http/")
        .replace("action='https://",  "action='crowbar-browser://proxy/https/")
        .replace("action='http://",   "action='crowbar-browser://proxy/http/")
}

fn inject_nav_script(html: &str) -> String {
    const SCRIPT: &str = r#"<script>
(function(){
  var _hist=[location.href], _idx=0;
  function realUrl(u){
    return u.replace(/^crowbar-browser:\/\/proxy\/https\//,'https://')
            .replace(/^crowbar-browser:\/\/proxy\/http\//,'http://');
  }
  function report(){
    window.parent.postMessage({
      type:'__crowbar_browser_nav__',
      url:realUrl(location.href),
      canGoBack:_idx>0,
      canGoForward:_idx<_hist.length-1
    },'*');
  }
  var _push=history.pushState.bind(history);
  history.pushState=function(s,t,u){
    _push(s,t,u);
    _hist=_hist.slice(0,_idx+1);
    _hist.push(location.href);
    _idx=_hist.length-1;
    report();
  };
  var _rep=history.replaceState.bind(history);
  history.replaceState=function(s,t,u){
    _rep(s,t,u);
    _hist[_idx]=location.href;
    report();
  };
  window.addEventListener('popstate',function(){
    var cur=location.href,pos=_hist.lastIndexOf(cur);
    if(pos>=0)_idx=pos; else{_idx=Math.max(0,_idx-1);}
    report();
  });
  window.addEventListener('load',report);
  window.addEventListener('message',function(e){
    if(!e.data||e.data.type!=='__crowbar_cmd__')return;
    if(e.data.cmd==='back')history.back();
    else if(e.data.cmd==='forward')history.forward();
    else if(e.data.cmd==='reload')location.reload();
  });
})();
</script>"#;

    if let Some(pos) = html.to_ascii_lowercase().find("</head>") {
        let mut out = html.to_string();
        out.insert_str(pos, SCRIPT);
        out
    } else {
        format!("{}{}", SCRIPT, html)
    }
}

fn shared_client() -> &'static reqwest::Client {
    static CLIENT: std::sync::OnceLock<reqwest::Client> = std::sync::OnceLock::new();
    CLIENT.get_or_init(|| {
        reqwest::Client::builder()
            .danger_accept_invalid_certs(false)
            .redirect(reqwest::redirect::Policy::limited(10))
            .build()
            .expect("reqwest client init failed")
    })
}

// ── Async scheme handler ──────────────────────────────────────────────────────

pub fn handle_request<R: Runtime>(
    _ctx: UriSchemeContext<'_, R>,
    request: http::Request<Vec<u8>>,
    responder: UriSchemeResponder,
) {
    let url_str = request.uri().to_string();

    tauri::async_runtime::spawn(async move {
        let resp = match fetch_and_transform(&url_str, &request).await {
            Ok(r) => r,
            Err(e) => http::Response::builder()
                .status(502)
                .header(http::header::CONTENT_TYPE, "text/plain")
                .body(format!("browser proxy error: {e}").into_bytes())
                .expect("static 502 response is valid"),
        };
        responder.respond(resp);
    });
}

async fn fetch_and_transform(
    proxy_url: &str,
    request: &http::Request<Vec<u8>>,
) -> Result<http::Response<Vec<u8>>, Box<dyn std::error::Error + Send + Sync>> {
    // SSRF note: this proxy is desktop-only (crowbar-browser:// is not network-exposed)
    // so we accept fetching local/private addresses. Block here if ever network-exposed.
    let real_url = parse_proxy_url(proxy_url)
        .ok_or("invalid crowbar-browser URL")?;

    let client = shared_client();

    let method = reqwest::Method::from_bytes(request.method().as_str().as_bytes())?;
    let mut req = client.request(method, &real_url);

    for (name, value) in request.headers() {
        let n = name.as_str().to_ascii_lowercase();
        if matches!(n.as_str(),
            "host" | "origin" | "referer" | "connection" |
            "transfer-encoding" | "keep-alive" | "upgrade" |
            "proxy-authorization" | "te" | "trailers"
        ) {
            continue;
        }
        req = req.header(name.as_str(), value.as_bytes());
    }

    if !request.body().is_empty() {
        req = req.body(request.body().clone());
    }

    let response = req.send().await?;
    let status = response.status().as_u16();
    let mut builder = http::Response::builder().status(status);

    for (name, value) in response.headers() {
        let n = name.as_str().to_ascii_lowercase();
        if n == "x-frame-options" {
            continue;
        }
        if n == "content-security-policy" || n == "content-security-policy-report-only" {
            let stripped = strip_csp_frame_ancestors(value.to_str().unwrap_or(""));
            if !stripped.is_empty() {
                builder = builder.header(name.as_str(), stripped);
            }
            continue;
        }
        if n == "transfer-encoding" {
            continue;
        }
        builder = builder.header(name.as_str(), value.as_bytes());
    }

    let content_type = response
        .headers()
        .get(reqwest::header::CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
        .to_ascii_lowercase();

    let bytes = response.bytes().await?;

    let body = if content_type.contains("text/html") {
        let html = String::from_utf8_lossy(&bytes);
        let transformed = inject_nav_script(&rewrite_links(&html));
        transformed.into_bytes()
    } else {
        bytes.to_vec()
    };

    Ok(builder.body(body)?)
}

// ── tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_https() {
        assert_eq!(
            parse_proxy_url("crowbar-browser://proxy/https/example.com/path?q=1"),
            Some("https://example.com/path?q=1".to_string())
        );
    }

    #[test]
    fn test_parse_http_with_port() {
        assert_eq!(
            parse_proxy_url("crowbar-browser://proxy/http/localhost:3000/api"),
            Some("http://localhost:3000/api".to_string())
        );
    }

    #[test]
    fn test_parse_invalid_protocol() {
        assert_eq!(parse_proxy_url("crowbar-browser://proxy/ftp/example.com"), None);
    }

    #[test]
    fn test_parse_wrong_scheme() {
        assert_eq!(parse_proxy_url("https://example.com"), None);
    }

    #[test]
    fn test_to_proxy_url_https() {
        assert_eq!(
            to_proxy_url("https://example.com/path"),
            Some("crowbar-browser://proxy/https/example.com/path".to_string())
        );
    }

    #[test]
    fn test_to_proxy_url_http() {
        assert_eq!(
            to_proxy_url("http://localhost:3000/"),
            Some("crowbar-browser://proxy/http/localhost:3000/".to_string())
        );
    }

    #[test]
    fn test_to_proxy_url_non_http() {
        assert_eq!(to_proxy_url("ftp://example.com"), None);
    }

    #[test]
    fn test_strip_csp_removes_frame_ancestors() {
        let csp = "default-src 'self'; frame-ancestors 'none'; script-src 'unsafe-eval'";
        let stripped = strip_csp_frame_ancestors(csp);
        assert!(!stripped.contains("frame-ancestors"));
        assert!(stripped.contains("default-src 'self'"));
        assert!(stripped.contains("script-src 'unsafe-eval'"));
    }

    #[test]
    fn test_strip_csp_no_frame_ancestors() {
        let csp = "default-src 'self'";
        assert_eq!(strip_csp_frame_ancestors(csp), "default-src 'self'");
    }

    #[test]
    fn test_strip_csp_empty() {
        assert_eq!(strip_csp_frame_ancestors(""), "");
    }

    #[test]
    fn test_rewrite_links_double_quote() {
        let html = r#"<a href="https://example.com/about">link</a>"#;
        let out = rewrite_links(html);
        assert!(out.contains("crowbar-browser://proxy/https/example.com/about"));
        assert!(!out.contains("href=\"https://"));
    }

    #[test]
    fn test_rewrite_links_single_quote() {
        let html = r#"<a href='http://example.com/'>link</a>"#;
        let out = rewrite_links(html);
        assert!(out.contains("crowbar-browser://proxy/http/example.com/"));
    }

    #[test]
    fn test_rewrite_links_form_action() {
        let html = r#"<form action="https://example.com/submit">"#;
        let out = rewrite_links(html);
        assert!(out.contains("crowbar-browser://proxy/https/example.com/submit"));
    }

    #[test]
    fn test_rewrite_links_preserves_relative() {
        let html = r#"<a href="/about">relative</a>"#;
        let out = rewrite_links(html);
        assert_eq!(out, html, "relative URLs must be left untouched");
    }

    #[test]
    fn test_inject_nav_script_before_closing_head() {
        let html = "<html><head><title>T</title></head><body></body></html>";
        let out = inject_nav_script(html);
        let script_pos = out.find("__crowbar_browser_nav__").unwrap();
        let head_close_pos = out.find("</head>").unwrap();
        assert!(script_pos < head_close_pos);
    }

    #[test]
    fn test_inject_nav_script_no_head_tag() {
        let html = "<div>bare</div>";
        let out = inject_nav_script(html);
        assert!(out.contains("__crowbar_browser_nav__"));
    }
}
