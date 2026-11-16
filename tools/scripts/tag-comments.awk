# Shared validation of tagged comments (manifest 7): SPEC:, PLATFORM:, SAFETY:, DEPRECATED:.
# Needs the caller's report(line, tag, msg) and the variable repo (repository root).
function tag_slug(h) { h = tolower(h); gsub(/[^a-z0-9 _-]/, "", h); gsub(/ /, "-", h); return h }
function tag_file_exists(path,   l) { if ((getline l < path) >= 0) { close(path); return 1 } return 0 }
function tag_anchor_exists(path, anchor,   l, h, found) {
  found = 0
  while ((getline l < path) > 0) {
    if (l ~ /^#+[ \t]/) { h = l; sub(/^#+[ \t]+/, "", h); sub(/[ \t]+$/, "", h); if (tag_slug(h) == anchor) found = 1 }
  }
  close(path)
  return found
}
function tag_platform(w) {
  w = tolower(w); gsub(/[^a-z0-9]/, "", w)
  return w ~ /^(macos|linux|ios|android|windows|wayland|x11|xwayland|gtk|webkit|wkwebview|webview|appkit|uikit|cocoa|metal|vulkan|tauri|xcode|darwin|unix|posix|swiftui|gpui|winit|safari|chrome|firefox|systemd|launchd)$/
}
function tag_words(s,   w) { return split(s, w, /[ \t]+/) }
function check_tag(text, line,   tag, body, first, path, anchor, k) {
  if (!match(text, /^(SPEC|PLATFORM|SAFETY|DEPRECATED):/)) return
  tag = substr(text, 1, RLENGTH - 1)
  body = substr(text, RLENGTH + 1); sub(/^[ \t]+/, "", body)
  split(body, k, /[ \t]+/); first = k[1]
  if (tag == "SPEC") {
    if (first ~ /^https?:\/\//) return
    sub(/[,;:.)]+$/, "", first)
    anchor = ""
    if (index(first, "#")) { anchor = substr(first, index(first, "#") + 1); first = substr(first, 1, index(first, "#") - 1) }
    path = repo "/" first
    if (first !~ /\// || !tag_file_exists(path)) { report(line, "spec-ref", "SPEC: must start with an existing repo path (docs/...#anchor) or a URL"); return }
    if (anchor != "" && !tag_anchor_exists(path, anchor)) report(line, "spec-ref", "SPEC: anchor #" anchor " not found in " first)
    return
  }
  if (tag == "PLATFORM") {
    if (!tag_platform(first) || tag_words(body) < 4) report(line, "platform-ref", "PLATFORM: needs a platform name (macos, linux, wayland, ...) then a reason of at least three words")
    return
  }
  if (tag_words(body) < 3) report(line, "tag-reason", tag ": needs a reason of at least three words")
}
