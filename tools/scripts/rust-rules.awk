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
function check_allow(buf, line,   inner, rs, lints, n, i, reason) {
  reason = ""
  if (match(buf, /reason[ \t]*=[ \t]*"[^"]*"/)) { reason = substr(buf, RSTART, RLENGTH); sub(/^reason[ \t]*=[ \t]*"/, "", reason); sub(/"$/, "", reason) }
  if (reason == "") report(line, "allow", "allow without reason = \"...\"")
  else if (length(reason) < 12) report(line, "allow", "allow reason must say why (at least 12 characters)")
  if (buf ~ /dead_code/) report(line, "allow-dead", "dead code is deleted, not allowed")
  inner = buf
  sub(/^.*(allow|expect)\(/, "", inner)
  gsub(/reason[ \t]*=[ \t]*"[^"]*"/, "", inner)
  gsub(/[()\[\]]/, "", inner)
  n = split(inner, lints, ",")
  for (i = 1; i <= n; i++) {
    gsub(/[ \t]/, "", lints[i])
    if (lints[i] != "" && lints[i] !~ /^[A-Za-z_:0-9]+$/) continue
    if (lints[i] == "unsafe_code") { unsafe_allow_line = line; continue }
    if (lints[i] != "") report(line, "allow-lint", lints[i] ": lint suppression needs an entry in tools/scripts/allow-baseline.txt (the list only shrinks)")
  }
}
function brace_delta(code,   o, c, t) { t = code; o = gsub(/\{/, "", t); t = code; c = gsub(/\}/, "", t); return o - c }
function track_trait(code,   d) {
  if (trait_name == "" && code ~ /^[ \t]*(pub(\([^)]*\))? )?(unsafe )?trait[ \t]+[A-Za-z_]/) {
    trait_name = code; sub(/^[ \t]*(pub(\([^)]*\))? )?(unsafe )?trait[ \t]+/, "", trait_name); match(trait_name, /^[A-Za-z0-9_]+/); trait_name = substr(trait_name, 1, RLENGTH)
    trait_line = NR; trait_fns = 0; trait_depth = 0; trait_open = 0
  } else if (trait_name == "") return
  if (trait_depth == 1 && code ~ /(^|[^A-Za-z_])fn[ \t]+[A-Za-z_]/) trait_fns++
  d = brace_delta(code)
  if (code ~ /\{/) trait_open = 1
  trait_depth += d
  if (trait_open && trait_depth <= 0) {
    if (trait_fns > 3) report(trait_line, "trait-size", "trait " trait_name " has " trait_fns " methods (max 3): split into small consumer-side traits")
    trait_name = ""
  }
}
function track_real_infra(raw,   reason) {
  # A fn carrying #[ignore = "real-infra: what and why"] (a test, or a helper only such tests call) is exempt in its
  # body, and so is any file under a `real_infra/` directory of test support; the CI job running --include-ignored runs them.
  if (raw ~ /#\[ignore[ \t]*=[ \t]*"real-infra:/) {
    reason = raw; sub(/^.*"real-infra:[ \t]*/, "", reason); sub(/".*$/, "", reason)
    if (tag_words(reason) >= 3) ign_pending = 1
    else report(NR, "real-infra", "#[ignore = \"real-infra: ...\"] needs a reason of at least three words (what and why)")
  }
  if (ign_pending && !ri_active && raw ~ /(^|[^A-Za-z_])fn[ \t]+[A-Za-z_]/) { ri_active = 1; ri_depth = 0; ri_open = 0 }
  if (raw ~ /(^|[^A-Za-z_])fn[ \t]+[A-Za-z_]/) ign_pending = 0
}
function close_real_infra(raw,   st) {
  if (!ri_active || ri_file + 0) return
  st = strip(raw)
  if (st ~ /\{/) ri_open = 1
  ri_depth += brace_delta(st)
  if (ri_open && ri_depth <= 0) ri_active = 0
}
function track_table(raw) {
  if (raw ~ /^[ \t]*\/\//) return
  if (raw ~ /(const|static)[ \t]+[A-Z_0-9]+[ \t]*:[ \t]*&?(\[|Vec|&\[)/ || raw ~ /let[ \t]+(mut[ \t]+)?[a-z_]*(cases|table|vectors|examples)[a-z_]*[ \t]*(:[^=]*)?=[ \t]*(&?\[|vec!\[)/) tbl_def = 1
  if (raw ~ /(^|[^A-Za-z_])for[ \t(].*[ \t)]in[ \t]+.*(CASES|cases|TABLE|table|VECTORS|vectors|\.iter\(\)|\.into_iter\(\)|&\[)/ || raw ~ /\.for_each\(/) tbl_iter = 1
  if (raw ~ /macro_rules!|#\[(rstest|test_case)|#\[case\(|test_case!/) tbl_macro = 1
}
function check_unsafe_home(raw,   u, i) {
  u = strip(raw); i = index(u, "//"); if (i) u = substr(u, 1, i - 1)
  if (u ~ /(^|[^A-Za-z_])unsafe([ \t]*[{(]|[ \t]+(fn|impl|extern|trait)([^A-Za-z_]|$))/)
    report(NR, "unsafe-home", "unsafe outside platform/src/{linux,macos}/: put it behind a safe API there (docs/rust-style.md, Crate layers)")
}
BEGIN { ri_active = ri_file + 0; pend_doc = 0; cfg_line = 0; in_t = 0; t_depth = 0; loc = 0 }
{
  raw = $0
  if (cfg_line) {
    if (raw ~ /^[ \t]*mod tests[ \t]*\{/ && mode == "src") { in_t = 1; t_depth = 0; has_tests = 1 }
    else if (raw !~ /^[ \t]*(pub(\([^)]*\))? )?mod test_support[ \t]*;/) report(cfg_line, "cfg-test", "#[cfg(test)] is only allowed on the inline `mod tests {` or `mod test_support;`")
    cfg_line = 0
  }
  if (raw ~ /#!?\[(allow|expect)\(/ || raw ~ /cfg_attr\(.*(allow|expect)\(/) { allow_buf = ""; allow_line = NR; collecting = 1 }
  if (collecting) {
    allow_buf = allow_buf raw
    if (raw ~ /\]/) {
      collecting = 0
      check_allow(allow_buf, allow_line)
    }
  }
  if (raw ~ /^[ \t]*#\[(tokio::)?test(\([^]]*\))?\]/ && (mode == "test" || in_t)) ntests++
  if (mode == "test" || in_t) track_table(raw)
  if ((mode == "test" || in_t) && raw ~ /start_paused|time::pause/) paused = 1
  if ((mode == "test" || in_t) && (raw ~ /time::sleep/ || raw ~ /(^|[^A-Za-z_:.])sleep\(/) && raw !~ /thread::sleep/ && raw !~ /^[ \t]*\/\//) { nsleep++; sleep_line[nsleep] = NR }
  if (mode == "test" || in_t) track_real_infra(raw)
  if ((mode == "test" || in_t) && !ri_active && raw !~ /^[ \t]*\/\// && raw !~ /(Fake|Mock|InMemory|Stub|Recording)[A-Za-z_]*/ &&
      raw ~ /thread::sleep|Tcp(Stream|Listener)|UdpSocket|Unix(Stream|Listener|Datagram)|Command::new|home_dir\(\)|env::var\("HOME"\)|env::temp_dir|tempfile|tempdir|TempDir|std::fs|tokio::fs|fs::(read|write|create_dir|remove|copy|rename|File|metadata|OpenOptions)|File::(open|create)|OpenOptions::new|reqwest::/)
    report(NR, "real-infra", "test touches real infrastructure (socket, fs, tempdir, process, sleep): use an in-memory fake, a channel or a paused clock")
  if (mode == "test" || in_t) close_real_infra(raw)
  if (!unsafe_home + 0) check_unsafe_home(raw)
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
  if (raw ~ /^[ \t]*#\[(tokio::)?test(\([^]]*\))?\]/) { if (!t_body) report(NR, "test-in-src", "#[test] outside the file's `mod tests`"); next }
  if (t_body) {
    s = strip(raw); i = index(s, "//")
    if (i) { text = substr(s, i + 2); sub(/^[ \t]+/, "", text); if (raw !~ /^[ \t]*\/\/\//) { if (text !~ /^(SAFETY|SPEC|PLATFORM|DEPRECATED):/) report(NR, "comment", "inline comment (only SAFETY:/SPEC:/PLATFORM:/DEPRECATED: allowed)"); else check_tag(text, NR); if (text ~ /^SAFETY:/) seen_safety = 1 } }
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
  if (s ~ /^[ \t]*(unsafe[ \t]+)?impl[ \t<]/) has_fn = 1
  if (s ~ /(^|[^A-Za-z_])fn[ \t]+[A-Za-z_0-9]+/) nfn++
  i = index(s, "//")
  if (i) {
    rest = substr(s, i)
    code = substr(s, 1, i - 1)
    text = substr(rest, 3)
    sub(/^[ \t]+/, "", text)
    if (text !~ /^(SAFETY|SPEC|PLATFORM|DEPRECATED):/) report(NR, "comment", "inline comment (only SAFETY:/SPEC:/PLATFORM:/DEPRECATED: allowed)")
    else { check_tag(text, NR); if (text ~ /^SAFETY:/) seen_safety = 1 }
  }
  if (s ~ /\/\*/) report(NR, "comment", "block comment")
  if (code ~ /\.unwrap\(\)/ && !allow_unwrap) report(NR, "panic", "unwrap() outside tests/main")
  if (code ~ /\.expect\(/ && !allow_unwrap) report(NR, "panic", "expect() outside tests/main")
  if (code ~ /(^|[^a-z_])(panic|todo|unimplemented|unreachable|dbg)!/) report(NR, "panic", "panic!/todo!/unimplemented!/unreachable!/dbg! outside tests")
  track_trait(code)
  if (code ~ /(^|[^A-Za-z_])(Instant|SystemTime)::now\(\)|(Utc|Local)::now\(\)/ && file !~ /\/adapters\//) report(NR, "direct-time", "direct clock read: inject a Clock (adapters/ directories are exempt)")
  if (code ~ /(^|[^A-Za-z_])(std|tokio)::fs([^A-Za-z_]|$)|(^|[^A-Za-z_:])fs::(read|write|create_dir|remove|copy|rename|metadata|File|OpenOptions)|File::(open|create)|OpenOptions::new/ && file !~ /\/adapters\//) report(NR, "direct-fs", "direct filesystem access: go through an injected port (adapters/ directories are exempt)")
  if (code ~ /Result<[^;{]*,[ \t]*(String|&('static[ \t]+)?str)[ \t]*>/) report(NR, "result-string", "Result with a String/&str error: use a typed error enum")
  if (code ~ /map_err\([ \t]*\|_[A-Za-z0-9_]*\|/) report(NR, "map-err-ignore", "map_err discards the source error: wrap it with context")
  if (code ~ /Err\(_\)[ \t]*(=>|=)|if let Err\(_\)|\.ok\(\);[ \t]*$/) report(NR, "swallowed-error", "error swallowed (Err(_) / .ok();): handle it, log it or return it")
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
  # An FFI boundary may allow unsafe_code (with a reason) without a baseline entry when its unsafe blocks carry SAFETY comments.
  if (unsafe_allow_line && !seen_safety) report(unsafe_allow_line, "allow-lint", "unsafe_code: allowed only in a file whose unsafe blocks carry // SAFETY: comments")
  if (!paused) for (i = 1; i <= nsleep; i++) report(sleep_line[i], "real-infra", "real sleep in a test: use a paused clock (start_paused) or a channel")
  table = (tbl_def && tbl_iter) || tbl_macro
  if (ntests > 2 && !table) report(1, "table-driven", ntests " #[test] fns and no table (a const array of cases iterated in one test, or a macro/rstest): one table, one behaviour per case")
  if (mode == "src" && loc > 500) report(1, "size", loc " non-test lines (max 500); split by concept")
  if (mode == "src" && nfn > 12) report(1, "fn-count", nfn " functions in one file (max 12): split by responsibility")
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
