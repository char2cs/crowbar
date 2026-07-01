# Iframe Browser Pane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the native child-webview overlay browser pane with an `<iframe>` backed by a Tauri URI scheme proxy that strips `X-Frame-Options` and `CSP frame-ancestors` headers, enabling any site to be embedded while keeping the browser fully inside the React/CSS layout.

**Architecture:** A new `crowbar-browser://proxy/https/example.com/path` URI scheme is registered in Tauri; the handler fetches the real URL with `reqwest`, strips framing-block headers, injects a small `postMessage` navigation-reporter script and rewrites absolute `href` links before returning the response. The React `WebViewer` component becomes a plain `<iframe src="crowbar-browser://...">` that listens for those `postMessage` events to keep the address bar and back/forward state current. Back/forward commands are sent *to* the iframe via `postMessage` (legal cross-origin); the injected script calls `history.back/forward()` in response.

**Tech Stack:** Rust `reqwest` 0.12 (new dep, native-tls), Tauri 2 `UriSchemeContext` / `UriSchemeResponder`, React `<iframe>`, `window.addEventListener('message')`.

---

## File Map

| Action | Path | Purpose |
|--------|------|---------|
| **Create** | `desktop/src-tauri/src/browser_proxy.rs` | Scheme handler: fetch, strip headers, inject script, rewrite links |
| **Modify** | `desktop/src-tauri/src/lib.rs` | Register `crowbar-browser` scheme alongside existing `crowbar` scheme |
| **Modify** | `desktop/src-tauri/Cargo.toml` | Add `reqwest` dependency |
| **Modify** | `web/src/features/web-viewer/components/web-viewer.tsx` | Replace overlay div + hook with `<iframe>` + postMessage handler |
| **Modify** | `web/src/features/workspace/components/workspace-view.tsx` | Remove `<BrowserPaneEventListener />` |
| **Modify** | `web/src/lib/crowbar-bridge.ts` | Remove all `browserPane*` bridge functions |
| **Delete** | `desktop/src-tauri/src/browser_pane.rs` | Native child-webview manager — replaced by proxy |
| **Delete** | `desktop/src-tauri/capabilities/browser-pane.json` | Capability only needed by native child webviews |
| **Delete** | `web/src/features/web-viewer/hooks/use-browser-pane-anchor.ts` | Overlay coordinate-sync hook — no longer needed |
| **Delete** | `web/src/features/web-viewer/components/browser-pane-event-listener.tsx` | Tauri-event nav listener — replaced by postMessage |
| **Delete** | `web/src/features/web-viewer/stores/web-viewer-navigation-store.ts` | Rebuild inline in the component (simpler local state) |
| **Delete** (tests) | `web/src/__tests__/features/web-viewer/hooks/use-browser-pane-anchor.test.ts` | Hook deleted |
| **Delete** (tests) | `web/src/__tests__/lib/crowbar-bridge-browser-pane.test.ts` | Functions deleted |
| **Update** (tests) | `web/src/__tests__/features/web-viewer/components/web-viewer.test.tsx` | Iframe assertions instead of overlay assertions |
| **Update** (tests) | `web/src/__tests__/features/web-viewer/stores/web-viewer-navigation-store.test.ts` | Delete (store moved inline) |

> **Why delete the navigation store?** The Zustand store was needed because the native webview's nav events arrived via Tauri IPC from a child webview and had to be broadcast app-wide. With `postMessage`, the events come directly into the `WebViewer` component's `window.addEventListener('message')`. Per-buffer nav state (`url`, `canGoBack`, `canGoForward`) fits cleanly in `useReducer` local state inside `WebViewer`. The `use-jump-navigation` hook currently reads `activeWebViewerNavigation` from the Zustand store — that integration should be dropped in this task (the tab-bar back/forward buttons already work via the editor jump list for non-webviewer buffers; webviewer-specific back/forward is surfaced in the WebViewer nav bar itself).

---

## Task 1: Rust Proxy Scheme Handler

**Files:**
- Create: `desktop/src-tauri/src/browser_proxy.rs`

---

- [ ] **Step 1.1 — Write failing unit tests for pure functions**

Create `desktop/src-tauri/src/browser_proxy.rs` with only the test module (no impl yet):

```rust
// desktop/src-tauri/src/browser_proxy.rs

// ── public surface (stubs so tests compile) ──────────────────────────────────

pub fn parse_proxy_url(_url: &str) -> Option<String> { None }
pub fn to_proxy_url(_url: &str) -> Option<String> { None }
fn strip_csp_frame_ancestors(_csp: &str) -> String { String::new() }
fn rewrite_links(_html: &str) -> String { String::new() }
fn inject_nav_script(_html: &str) -> String { String::new() }

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
```

- [ ] **Step 1.2 — Run tests, confirm they all fail**

```bash
cd desktop/src-tauri && cargo test browser_proxy 2>&1 | tail -20
```

Expected: multiple FAILED lines, one per unimplemented function.

- [ ] **Step 1.3 — Implement the pure functions**

Replace the stub implementations in `browser_proxy.rs` with the real ones (keep the `#[cfg(test)]` block as-is):

```rust
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

/// Rewrite absolute https:// and http:// hrefs/actions so navigation stays
/// inside the proxy. Relative URLs resolve correctly on their own.
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

/// Inject the navigation reporter + command listener before </head>.
/// Injected into every HTML response so the parent React frame stays in sync.
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
  // Commands from parent (back / forward / reload)
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
```

- [ ] **Step 1.4 — Run tests again, confirm they all pass**

```bash
cd desktop/src-tauri && cargo test browser_proxy 2>&1 | tail -20
```

Expected: `test result: ok. 15 passed; 0 failed`

- [ ] **Step 1.5 — Add the async scheme handler**

Add these imports at the top of `browser_proxy.rs` and append the `handle_request` function after the pure functions (before the `#[cfg(test)]` block):

```rust
use tauri::{Runtime, UriSchemeContext, UriSchemeResponder};
use tauri::http;

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
                .unwrap(),
        };
        responder.respond(resp);
    });
}

async fn fetch_and_transform(
    proxy_url: &str,
    request: &http::Request<Vec<u8>>,
) -> Result<http::Response<Vec<u8>>, Box<dyn std::error::Error + Send + Sync>> {
    let real_url = parse_proxy_url(proxy_url)
        .ok_or("invalid crowbar-browser URL")?;

    let client = reqwest::Client::builder()
        .danger_accept_invalid_certs(false)
        .redirect(reqwest::redirect::Policy::limited(10))
        .build()?;

    // Forward method and a safe subset of request headers.
    let method = reqwest::Method::from_bytes(request.method().as_str().as_bytes())?;
    let mut req = client.request(method, &real_url);

    for (name, value) in request.headers() {
        // Drop hop-by-hop and origin headers that would confuse remote servers.
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
        // Strip framing-block headers.
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
        // Drop transfer-encoding; we're buffering the whole body.
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
```

- [ ] **Step 1.6 — Confirm it compiles (no handler wired yet — that's Task 2)**

```bash
cd desktop/src-tauri && cargo check 2>&1 | grep -E "^error" | head -20
```

Expected: errors about `reqwest` not found (that's expected — Cargo.toml change is in Task 2). If there are _other_ errors, fix them first.

- [ ] **Step 1.7 — Commit**

```bash
git add desktop/src-tauri/src/browser_proxy.rs
git commit -m "feat(browser): add crowbar-browser:// proxy scheme handler (Rust)"
```

---

## Task 2: Wire Proxy Into Tauri, Add reqwest

**Files:**
- Modify: `desktop/src-tauri/Cargo.toml`
- Modify: `desktop/src-tauri/src/lib.rs`

---

- [ ] **Step 2.1 — Add reqwest to Cargo.toml**

Open `desktop/src-tauri/Cargo.toml`. In the `[dependencies]` section, add after the `log` line:

```toml
reqwest = { version = "0.12", default-features = false, features = ["native-tls"] }
```

- [ ] **Step 2.2 — Register the new module and scheme in lib.rs**

In `desktop/src-tauri/src/lib.rs`, add `mod browser_proxy;` alongside the existing module declarations at the top:

```rust
mod api_proxy;
mod browser_pane;
mod browser_proxy;   // ← add this line
mod sidecar;
mod terminal;
```

Then, in the `run()` function, add the new scheme registration **after** the existing `crowbar` scheme:

```rust
        .register_asynchronous_uri_scheme_protocol("crowbar", api_proxy::handle_request)
        .register_asynchronous_uri_scheme_protocol("crowbar-browser", browser_proxy::handle_request);
```

> **Note:** `browser_pane` is intentionally left registered for now — removing it while the old `WebViewer` component still references the bridge functions would cause bridge call errors at runtime. It'll be cleaned up in Task 4.

- [ ] **Step 2.3 — Verify it compiles**

```bash
cd desktop/src-tauri && cargo check 2>&1 | grep -E "^error" | head -20
```

Expected: zero errors. `reqwest` will be downloaded on first run.

- [ ] **Step 2.4 — Run existing Rust tests to confirm nothing regressed**

```bash
cd desktop/src-tauri && cargo test 2>&1 | tail -10
```

Expected: all existing tests pass plus 15 new `browser_proxy` tests.

- [ ] **Step 2.5 — Commit**

```bash
git add desktop/src-tauri/Cargo.toml desktop/src-tauri/src/lib.rs
git commit -m "feat(browser): register crowbar-browser:// URI scheme in Tauri"
```

---

## Task 3: Iframe WebViewer Component

This task replaces the entire `WebViewer` component. The old component used a native child webview overlay; the new one is a plain `<iframe>`. Navigation state (url, canGoBack, canGoForward) moves from the Zustand store into `useReducer` local state — simpler and correct since no other component needs it.

**Files:**
- Modify: `web/src/features/web-viewer/components/web-viewer.tsx`
- Modify: `web/src/features/tabs/hooks/use-jump-navigation.ts` (remove webviewer nav dependency)
- Modify: `web/src/__tests__/features/web-viewer/components/web-viewer.test.tsx`

---

- [ ] **Step 3.1 — Write the new WebViewer test first**

Replace `web/src/__tests__/features/web-viewer/components/web-viewer.test.tsx` entirely:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: vi.fn().mockReturnValue(false) }))

import { WebViewer } from '@/features/web-viewer/components/web-viewer'

describe('WebViewer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders an iframe with a crowbar-browser src in Tauri mode', () => {
    const { isTauri } = await import('@/lib/crowbar-bridge')
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(true)

    render(<WebViewer url="https://example.com" bufferId="b1" isVisible />)

    const iframe = document.querySelector('iframe')
    expect(iframe).toBeTruthy()
    expect(iframe?.src).toContain('crowbar-browser://proxy/https/example.com')
  })

  it('renders an iframe with the direct URL in non-Tauri mode (dev fallback)', () => {
    render(<WebViewer url="https://example.com" bufferId="b1" isVisible />)

    const iframe = document.querySelector('iframe')
    expect(iframe).toBeTruthy()
    expect(iframe?.src).toContain('example.com')
  })

  it('shows the URL in the address bar', () => {
    render(<WebViewer url="https://example.com" bufferId="b1" isVisible />)
    const input = screen.getByPlaceholderText('Enter URL or search…')
    expect(input).toHaveValue('https://example.com')
  })

  it('updates address bar when postMessage nav event arrives', async () => {
    render(<WebViewer url="https://example.com" bufferId="b1" isVisible />)

    await act(async () => {
      window.dispatchEvent(
        new MessageEvent('message', {
          data: {
            type: '__crowbar_browser_nav__',
            url: 'https://example.com/about',
            canGoBack: true,
            canGoForward: false,
          },
        }),
      )
    })

    const input = screen.getByPlaceholderText('Enter URL or search…')
    expect(input).toHaveValue('https://example.com/about')
  })

  it('normalizes bare domain in address bar on submit', async () => {
    const user = userEvent.setup()
    render(<WebViewer url="about:blank" bufferId="b1" isVisible />)

    const input = screen.getByPlaceholderText('Enter URL or search…')
    await user.clear(input)
    await user.type(input, 'github.com')
    await user.keyboard('{Enter}')

    expect(input).toHaveValue('https://github.com')
  })

  it('falls back to Google search for non-URL input', async () => {
    const user = userEvent.setup()
    render(<WebViewer url="about:blank" bufferId="b1" isVisible />)

    const input = screen.getByPlaceholderText('Enter URL or search…')
    await user.clear(input)
    await user.type(input, 'how does react work')
    await user.keyboard('{Enter}')

    expect(input).toHaveValue(
      'https://www.google.com/search?q=how%20does%20react%20work',
    )
  })
})
```

- [ ] **Step 3.2 — Run the new tests to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/web-viewer/components/web-viewer.test.tsx 2>&1 | tail -20
```

Expected: multiple FAILED — the component still has the old implementation.

- [ ] **Step 3.3 — Replace web-viewer.tsx**

Replace `web/src/features/web-viewer/components/web-viewer.tsx` entirely:

```tsx
import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import {
  ArrowLeft,
  ArrowRight,
  ArrowClockwise as RotateCw,
  GlobeHemisphereWest as Globe,
} from '@phosphor-icons/react'
import { cn } from '@/utils/cn'
import { isTauri } from '@/lib/crowbar-bridge'

export interface WebViewerProps {
  url?: string
  bufferId?: string
  isActive?: boolean
  isVisible?: boolean
  [key: string]: unknown
}

// ── URL helpers ───────────────────────────────────────────────────────────────

function normalizeUrl(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return 'about:blank'
  if (trimmed.startsWith('about:')) return trimmed
  if (/^https?:\/\//i.test(trimmed)) return trimmed
  if (/^[^\s/]+\.[^\s/]+/.test(trimmed)) return `https://${trimmed}`
  return `https://www.google.com/search?q=${encodeURIComponent(trimmed)}`
}

function toProxySrc(url: string): string {
  if (url === 'about:blank') return 'about:blank'
  if (!isTauri()) return url // dev fallback: direct iframe (no proxy available)
  if (url.startsWith('https://')) return `crowbar-browser://proxy/https/${url.slice(8)}`
  if (url.startsWith('http://')) return `crowbar-browser://proxy/http/${url.slice(7)}`
  return url
}

// ── Nav state ─────────────────────────────────────────────────────────────────

interface NavState {
  url: string
  canGoBack: boolean
  canGoForward: boolean
}

type NavAction =
  | { type: 'navigate'; url: string; canGoBack: boolean; canGoForward: boolean }
  | { type: 'setUrl'; url: string }

function navReducer(state: NavState, action: NavAction): NavState {
  switch (action.type) {
    case 'navigate':
      return { url: action.url, canGoBack: action.canGoBack, canGoForward: action.canGoForward }
    case 'setUrl':
      return { ...state, url: action.url }
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

export function WebViewer({
  url: initialUrl = 'about:blank',
  bufferId = '',
  isActive,
  isVisible = true,
}: WebViewerProps) {
  const normalizedInitial = normalizeUrl(initialUrl)
  const iframeRef = useRef<HTMLIFrameElement>(null)

  const [nav, dispatch] = useReducer(navReducer, {
    url: normalizedInitial,
    canGoBack: false,
    canGoForward: false,
  })

  const [inputValue, setInputValue] = useState(normalizedInitial)

  // Sync address bar from navigation events (pushed by the injected script)
  useEffect(() => {
    setInputValue(nav.url)
  }, [nav.url])

  // Listen for postMessage events from the injected script in the iframe
  useEffect(() => {
    function handleMessage(e: MessageEvent) {
      // Only accept messages that look like our nav events.
      // We can't check e.source === iframe.contentWindow cross-origin, so we
      // rely on the distinctive type string and ignore anything else.
      if (!e.data || e.data.type !== '__crowbar_browser_nav__') return
      const { url, canGoBack, canGoForward } = e.data as {
        url: string
        canGoBack: boolean
        canGoForward: boolean
      }
      dispatch({ type: 'navigate', url, canGoBack, canGoForward })
    }
    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [])

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      const normalized = normalizeUrl(inputValue)
      setInputValue(normalized)
      dispatch({ type: 'setUrl', url: normalized })
      if (iframeRef.current) {
        iframeRef.current.src = toProxySrc(normalized)
      }
    },
    [inputValue],
  )

  const sendCmd = useCallback((cmd: 'back' | 'forward' | 'reload') => {
    iframeRef.current?.contentWindow?.postMessage({ type: '__crowbar_cmd__', cmd }, '*')
  }, [])

  return (
    <div className={cn('flex h-full flex-col overflow-hidden', !isActive && 'pointer-events-none')}>
      {/* Navigation bar */}
      <form
        onSubmit={handleSubmit}
        className="flex shrink-0 items-center gap-1 border-b border-border bg-card px-2 py-1.5"
      >
        <button
          type="button"
          title="Back"
          disabled={!nav.canGoBack}
          onClick={() => sendCmd('back')}
          className="flex items-center justify-center rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
        >
          <ArrowLeft size={14} />
        </button>
        <button
          type="button"
          title="Forward"
          disabled={!nav.canGoForward}
          onClick={() => sendCmd('forward')}
          className="flex items-center justify-center rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
        >
          <ArrowRight size={14} />
        </button>
        <button
          type="button"
          title="Reload"
          onClick={() => sendCmd('reload')}
          className="flex items-center justify-center rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <RotateCw size={14} />
        </button>

        <div className="flex min-w-0 flex-1 items-center gap-1.5 rounded-md bg-background px-2 py-1 ring-1 ring-border focus-within:ring-primary">
          <Globe size={12} className="shrink-0 text-muted-foreground" />
          <input
            type="text"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onFocus={(e) => e.target.select()}
            placeholder="Enter URL or search…"
            className="min-w-0 flex-1 bg-transparent text-xs text-foreground outline-none placeholder:text-muted-foreground"
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="off"
          />
        </div>
      </form>

      {/* Browser iframe */}
      <iframe
        ref={iframeRef}
        src={toProxySrc(normalizedInitial)}
        className="min-h-0 flex-1 w-full border-0 bg-background"
        title="Browser"
        // Prevent the iframe from navigating the top-level Tauri window.
        // allow-same-origin keeps the injected script's sessionStorage working.
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-downloads allow-modals"
      />
    </div>
  )
}

export default WebViewer
```

> **Phosphor icons used:** `ArrowLeft`, `ArrowRight` — confirm these exist in `@phosphor-icons/react`. If not, use `CaretLeft` / `CaretRight` or any available directional icon.

- [ ] **Step 3.4 — Run the new tests**

```bash
cd web && npx vitest run src/__tests__/features/web-viewer/components/web-viewer.test.tsx 2>&1 | tail -20
```

Expected: all 6 tests pass.

- [ ] **Step 3.5 — Remove webviewer nav dependency from use-jump-navigation**

Open `web/src/features/tabs/hooks/use-jump-navigation.ts`. Find and remove any reference to `activeWebViewerNavigation` or `useWebViewerNavigationStore`. The hook should only handle editor jump-list navigation. If it currently has a branch for webViewer type buffers that calls `goBack`/`goForward` from the nav store, remove that branch entirely — back/forward for the iframe is handled inside the WebViewer component's nav bar.

- [ ] **Step 3.6 — Run the full web-viewer test suite**

```bash
cd web && npx vitest run src/__tests__/features/web-viewer/ 2>&1 | tail -20
```

Expected: tests in `components/` pass. Tests in `stores/` and `hooks/` will be deleted in Task 4 — skip for now.

- [ ] **Step 3.7 — Commit**

```bash
git add web/src/features/web-viewer/components/web-viewer.tsx \
        web/src/features/tabs/hooks/use-jump-navigation.ts \
        web/src/__tests__/features/web-viewer/components/web-viewer.test.tsx
git commit -m "feat(browser): replace native-overlay WebViewer with iframe + proxy"
```

---

## Task 4: Cleanup — Remove All Overlay Infrastructure

This task deletes the native child webview system: the Rust module, the capabilities file, the Zustand nav store, the anchor hook, the event listener, and the corresponding bridge functions and tests.

**Files:**
- Delete: `desktop/src-tauri/src/browser_pane.rs`
- Delete: `desktop/src-tauri/capabilities/browser-pane.json`
- Delete: `web/src/features/web-viewer/hooks/use-browser-pane-anchor.ts`
- Delete: `web/src/features/web-viewer/components/browser-pane-event-listener.tsx`
- Delete: `web/src/features/web-viewer/stores/web-viewer-navigation-store.ts`
- Delete: `web/src/__tests__/features/web-viewer/hooks/use-browser-pane-anchor.test.ts`
- Delete: `web/src/__tests__/features/web-viewer/stores/web-viewer-navigation-store.test.ts`
- Delete: `web/src/__tests__/lib/crowbar-bridge-browser-pane.test.ts`
- Modify: `desktop/src-tauri/src/lib.rs` (remove browser_pane wiring)
- Modify: `web/src/lib/crowbar-bridge.ts` (remove browserPane* functions)
- Modify: `web/src/features/workspace/components/workspace-view.tsx` (remove event listener mount)

---

- [ ] **Step 4.1 — Remove browser_pane from lib.rs**

In `desktop/src-tauri/src/lib.rs`:

1. Delete the `mod browser_pane;` line.
2. Delete `.manage(browser_pane::BrowserPaneManager::new())`.
3. Remove all `browser_pane::*` entries from `.invoke_handler(tauri::generate_handler![...])`.

- [ ] **Step 4.2 — Delete Rust and capability files**

```bash
rm desktop/src-tauri/src/browser_pane.rs
rm desktop/src-tauri/capabilities/browser-pane.json
```

- [ ] **Step 4.3 — Confirm Rust still compiles**

```bash
cd desktop/src-tauri && cargo check 2>&1 | grep -E "^error" | head -20
```

Expected: zero errors.

- [ ] **Step 4.4 — Remove browserPane* functions from crowbar-bridge.ts**

Open `web/src/lib/crowbar-bridge.ts`. Delete the following exported functions entirely:
- `browserPaneSync`
- `browserPaneNavigate`
- `browserPaneGoBack`
- `browserPaneGoForward`
- `browserPaneReload`
- `browserPaneClose`

- [ ] **Step 4.5 — Delete frontend files**

```bash
rm web/src/features/web-viewer/hooks/use-browser-pane-anchor.ts
rm web/src/features/web-viewer/components/browser-pane-event-listener.tsx
rm web/src/features/web-viewer/stores/web-viewer-navigation-store.ts
```

- [ ] **Step 4.6 — Remove BrowserPaneEventListener from workspace-view.tsx**

In `web/src/features/workspace/components/workspace-view.tsx`:
1. Delete the import of `BrowserPaneEventListener`.
2. Delete `<BrowserPaneEventListener />` from the JSX.

- [ ] **Step 4.7 — Delete obsolete test files**

```bash
rm web/src/__tests__/features/web-viewer/hooks/use-browser-pane-anchor.test.ts
rm web/src/__tests__/features/web-viewer/stores/web-viewer-navigation-store.test.ts
rm web/src/__tests__/lib/crowbar-bridge-browser-pane.test.ts
```

- [ ] **Step 4.8 — Fix any remaining TypeScript errors**

```bash
cd web && npx tsc --noEmit 2>&1 | head -40
```

Fix any "cannot find module" or "property does not exist" errors caused by deleted exports. Common spots: imports of `useWebViewerNavigationStore` anywhere outside the now-deleted store file, any remaining import of `browserPaneSync` etc.

```bash
grep -r "useWebViewerNavigationStore\|browserPaneSync\|browserPaneNavigate\|browserPaneClose\|BrowserPaneEventListener\|useBrowserPaneAnchor" web/src --include="*.ts" --include="*.tsx"
```

Expected: zero results. Fix anything that shows up.

- [ ] **Step 4.9 — Run all web-viewer tests**

```bash
cd web && npx vitest run src/__tests__/features/web-viewer/ 2>&1 | tail -20
```

Expected: only `web-viewer.test.tsx` exists (the other two were deleted) — all 6 pass.

- [ ] **Step 4.10 — Run full test suite**

```bash
cd web && npx vitest run 2>&1 | tail -15
```

Expected: all tests pass, zero failures.

- [ ] **Step 4.11 — Commit**

```bash
git add -A
git commit -m "refactor(browser): remove native child-webview overlay, bridge fns, and dead tests"
```

---

## Self-Review Checklist

**Spec coverage:**
- ✅ Proxy handler strips `X-Frame-Options` → Task 1
- ✅ Proxy handler strips `CSP frame-ancestors` → Task 1
- ✅ Navigation reporter injected into HTML responses → Task 1
- ✅ Absolute links rewritten to stay in proxy → Task 1
- ✅ Scheme registered in Tauri → Task 2
- ✅ `<iframe>` replaces overlay div → Task 3
- ✅ Address bar updates from postMessage events → Task 3
- ✅ Back/forward/reload via postMessage commands → Task 3
- ✅ URL normalization (bare domain → https, text → Google) → Task 3
- ✅ All native overlay code removed → Task 4
- ✅ Tests added/updated at each step → Tasks 1, 3
- ✅ Tests deleted for removed code → Task 4

**No placeholders:** Every step contains actual code or exact shell commands.

**Type consistency:** `NavState` / `NavAction` / `dispatch` defined and used within Task 3's single component file. `parse_proxy_url` / `to_proxy_url` defined in Task 1 and referenced in Task 1's handler — no cross-task type dependencies.

---

## On the Future "Point and Tell" System

**Is it achievable? Yes — and the iframe architecture makes it significantly more tractable than the native overlay.**

Because all HTML responses go through the Rust proxy, you have two insertion points:

1. **In the injected script (already there):** extend it to support a "select element" mode. When active, hover events highlight elements and clicking serializes the element's DOM path, tag, text content, and computed styles, then `postMessage`s the result to the parent frame. The parent (React) receives it and gives it to the agent as context.

2. **In the Rust proxy (request logging):** since every sub-resource request goes through the scheme handler, the proxy can log all network activity (URL, method, status, size, timing) into an in-memory ring buffer. An agent can query this to understand what failed — without needing the Chrome DevTools protocol.

The combination gives an agent:
- **What the user sees** — the serialized element under the cursor
- **Why it's broken** — the network log (failed requests, 4xx/5xx, CORS errors)
- **Where in the code** — Crowbar already knows what file is open in the editor

This is genuinely more useful than the Chrome MCP for Crowbar's use case because the context is co-located with the code. The Chrome MCP requires a separate browser, separate tab management, and has no awareness of what the developer is currently editing. Crowbar's integrated browser + agent + editor can answer "why is this button broken" by correlating the failing network request with the file that makes that call — something no external tool can do.
