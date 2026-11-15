# Per-file Rust rule scanner used by check-rust-rules.sh.
# vars: file (display path), mode (src|test), stem, allow_unwrap (0|1), allow_types (0|1)
function report(line, tag, msg) { printf "%s:%d: [%s] %s\n", file, line, tag, msg; bad = 1 }
function strip(s) {
  gsub(/r#+"[^\n]*"#+/, "\"\"", s)
  gsub(/\\"/, "", s)
  gsub(/"[^"]*"/, "\"\"", s)
  gsub(/'.'/, "''", s)
  return s
}
function snake(s) {
  gsub(/[a-z0-9][A-Z]/, "&", s)
  while (match(s, /[a-z0-9][A-Z]/)) s = substr(s, 1, RSTART) "_" substr(s, RSTART + 1)
  while (match(s, /[A-Z][A-Z][a-z]/)) s = substr(s, 1, RSTART) "_" substr(s, RSTART + 1)
  return tolower(s)
}
BEGIN { pend_doc = 0; cfg_line = 0; in_t = 0; t_depth = 0; loc = 0 }
{
  raw = $0
  if (cfg_line) {
    if (raw ~ /^[ \t]*mod tests[ \t]*\{/ && mode == "src") { in_t = 1; t_depth = 0; has_tests = 1 }
    else if (raw !~ /^[ \t]*(pub(\([^)]*\))? )?mod test_support[ \t]*;/) report(cfg_line, "cfg-test", "#[cfg(test)] is only allowed on the inline `mod tests {` or `mod test_support;`")
    cfg_line = 0
  }
  if (raw ~ /#!?\[allow\(/) { allow_buf = ""; allow_line = NR; collecting = 1 }
  if (collecting) {
    allow_buf = allow_buf raw
    if (raw ~ /\]/) {
      collecting = 0
      if (allow_buf !~ /reason *=/) report(allow_line, "allow", "allow without reason = \"...\"")
      if (allow_buf ~ /dead_code/) report(allow_line, "allow-dead", "dead code is deleted, not allowed")
    }
  }
  if (raw ~ /^[ \t]*#\[(tokio::)?test\]/ && (mode == "test" || in_t)) ntests++
  if ((mode == "test" || in_t) && raw ~ /CASES|cases/) table = 1
  if ((mode == "test" || in_t) && raw ~ /start_paused|time::pause/) paused = 1
  if ((mode == "test" || in_t) && raw ~ /time::sleep/ && raw !~ /^[ \t]*\/\//) { nsleep++; sleep_line[nsleep] = NR }
  if ((mode == "test" || in_t) && raw ~ /thread::sleep|TcpStream::connect|UdpSocket::bind|Command::new|home_dir\(\)|env::var\("HOME"\)|std::net::UdpSocket/ && raw !~ /^[ \t]*\/\//)
    report(NR, "real-infra", "test touches real infrastructure: use an in-memory fake, a channel or a paused clock")
  if (mode == "test") next
  if (in_t) {
    st = strip(raw)
    t_depth += gsub(/\{/, "{", st) - gsub(/\}/, "}", st)
    if (t_depth <= 0) in_t = 0
    t_body = 1
  } else {
    t_body = 0
    if (raw !~ /^[ \t]*$/ && raw !~ /^[ \t]*\/\/[!\/]/) loc++
  }
  if (!t_body && raw ~ /^[ \t]*#\[cfg\(.*test/ && raw !~ /not\(test\)/) { cfg_line = NR; next }
  if (raw ~ /^[ \t]*#\[path *=/ || raw ~ /^[ \t]*mod tests[ \t]*;/) { report(NR, "test-sibling", "tests live in an inline `mod tests {}` in this file, not a sibling *_tests.rs"); next }
  if (raw ~ /^[ \t]*#\[(tokio::)?test\]/) { if (!t_body) report(NR, "test-in-src", "#[test] outside the file's `mod tests`"); next }
  if (t_body) {
    s = strip(raw); i = index(s, "//")
    if (i) { text = substr(s, i + 2); sub(/^[ \t]+/, "", text); if (text !~ /^(SAFETY|SPEC|PLATFORM|DEPRECATED):/ && raw !~ /^[ \t]*\/\/\//) report(NR, "comment", "inline comment (only SAFETY:/SPEC:/PLATFORM:/DEPRECATED: allowed)") }
    next
  }
  if (raw ~ /^[ \t]*\/\/\//) { if (!pend_doc) doc_line = NR; pend_doc = 1; next }
  if (raw ~ /^[ \t]*\/\/!/) next
  if (raw ~ /^[ \t]*#\[/ || raw ~ /^[ \t]*$/) { if (raw ~ /^[ \t]*$/) pend_doc = 0; next }
  if (pend_doc) {
    if (raw ~ /^(pub\((crate|super|in [a-z_:]+)\) )?(async |const |unsafe |extern "C" )*(fn|struct|enum|trait|type|const|static|mod|union)[ \t]/ ||
        raw ~ /^[ \t]+pub\((crate|super)\) /)
      report(doc_line, "doc-private", "doc comment on a non-public item")
    pend_doc = 0
  }
  s = strip(raw)
  code = s
  if (s ~ /^[ \t]*\)[^;{]*\{[ \t]*$/ || s ~ /(^|[^A-Za-z_])fn[ \t]+[A-Za-z_][^;]*\{/) has_fn = 1
  i = index(s, "//")
  if (i) {
    rest = substr(s, i)
    code = substr(s, 1, i - 1)
    text = substr(rest, 3)
    sub(/^[ \t]+/, "", text)
    if (text !~ /^(SAFETY|SPEC|PLATFORM|DEPRECATED):/) report(NR, "comment", "inline comment (only SAFETY:/SPEC:/PLATFORM:/DEPRECATED: allowed)")
  }
  if (s ~ /\/\*/) report(NR, "comment", "block comment")
  if (code ~ /\.unwrap\(\)/ && !allow_unwrap) report(NR, "panic", "unwrap() outside tests/main")
  if (code ~ /\.expect\(/ && !allow_unwrap) report(NR, "panic", "expect() outside tests/main")
  if (code ~ /(^|[^a-z_])(panic|todo|unimplemented|dbg)!/) report(NR, "panic", "panic!/todo!/unimplemented!/dbg! outside tests")
  if (code ~ /static mut |lazy_static!|thread_local!|#\[ctor/ ||
      code ~ /^[ \t]*(pub(\([a-z]+\))? )?static [A-Z_0-9]+:.*(Mutex|RwLock|RefCell|Atomic|OnceLock|OnceCell|LazyLock|Cell)/)
    report(NR, "global", "global mutable state or init magic")
  if (code ~ /^(pub(\([^)]*\))? )?(struct|enum|trait|union)[ \t]+[A-Za-z_]/) {
    t = code
    sub(/^(pub(\([^)]*\))? )?(struct|enum|trait|union)[ \t]+/, "", t)
    match(t, /^[A-Za-z0-9_]+/)
    n++; names[n] = substr(t, 1, RLENGTH)
  }
}
END {
  if (!paused) for (i = 1; i <= nsleep; i++) report(sleep_line[i], "real-infra", "real sleep in a test: use a paused clock (start_paused) or a channel")
  if (ntests > 2 && !table) report(1, "table-driven", ntests " #[test] fns and no CASES table: one table, one behaviour per case")
  if (mode == "src" && loc > 500) report(1, "size", loc " non-test lines (max 500); split by concept")
  if (mode == "src" && !has_tests && has_fn && !exempt + 0 && !is_entry + 0) report(1, "test-missing", "source file with functions needs an inline #[cfg(test)] mod tests")
  if (mode == "src" && !allow_types) {
    if (n > 1) report(1, "multi-type", n " top-level types in one file: " names[1] ", " names[2] (n > 2 ? ", ..." : ""))
    if (n == 1) {
      ok = (snake(names[1]) == stem) || (stem == "error" && names[1] ~ /Error$/)
      if (!ok) report(1, "type-name", "type " names[1] " must live in " snake(names[1]) ".rs")
    }
  }
  exit bad ? 1 : 0
}
