# Per-file Rust rule scanner used by check-rust-rules.sh.
# vars: file (display path), mode (src|test), stem, allow_unwrap (0|1), allow_types (0|1)
function report(line, tag, msg) { printf "%s:%d: [%s] %s\n", file, line, tag, msg; bad = 1 }
function strip(s) {
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
BEGIN { pend_doc = 0; cfg_line = 0 }
{
  raw = $0
  if (cfg_line && raw !~ /^[ \t]*#\[path *= *"[a-z0-9_]+_tests\.rs"\]/) {
    report(cfg_line, "cfg-test", "#[cfg(test)] is only allowed as the wiring of foo_tests.rs (#[path] on the next line)")
  }
  if (cfg_line) cfg_line = 0
  if (raw ~ /#!?\[allow\(/) {
    if (raw !~ /reason *=/) report(NR, "allow", "allow without reason = \"...\"")
    if (raw ~ /dead_code/) report(NR, "allow-dead", "dead code is deleted, not allowed")
  }
  if (mode == "test") next
  if (raw ~ /^[ \t]*#\[cfg\(.*test/ && raw !~ /not\(test\)/) { cfg_line = NR; next }
  if (raw ~ /^[ \t]*#\[(tokio::)?test\]/) { report(NR, "test-in-src", "#[test] in a source file"); next }
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
  if (code ~ /^[ \t]*mod +tests *\{/) report(NR, "test-inline", "inline mod tests; move to foo_tests.rs")
  if (code ~ /^(pub(\([^)]*\))? )?(struct|enum|trait|union)[ \t]+[A-Za-z_]/) {
    t = code
    sub(/^(pub(\([^)]*\))? )?(struct|enum|trait|union)[ \t]+/, "", t)
    match(t, /^[A-Za-z0-9_]+/)
    n++; names[n] = substr(t, 1, RLENGTH)
  }
}
END {
  if (mode == "src" && !allow_types) {
    if (n > 1) report(1, "multi-type", n " top-level types in one file: " names[1] ", " names[2] (n > 2 ? ", ..." : ""))
    if (n == 1) {
      ok = (snake(names[1]) == stem) || (stem == "error" && names[1] ~ /Error$/)
      if (!ok) report(1, "type-name", "type " names[1] " must live in " snake(names[1]) ".rs")
    }
  }
  exit bad ? 1 : 0
}
