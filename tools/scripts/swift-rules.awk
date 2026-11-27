# Per-file Swift rule scanner used by check-swift-rules.sh.
# vars: file, mode (src|test), stem
function report(line, tag, msg) { printf "%s:%d: [%s] %s\n", file, line, tag, msg; bad = 1 }
function strip(s) { gsub(/\\"/, "", s); gsub(/"[^"]*"/, "\"\"", s); return s }
{
  raw = $0
  if (raw ~ /swiftlint:disable|swift-format-ignore/ && raw !~ / - /) report(NR, "lint-reason", "lint suppression without ' - reason'")
  if (mode == "test") {
    if (raw ~ /^[ \t]*func test[A-Za-z0-9_]*\(/) ntests++
    tmp = raw; gsub(/allCases/, "", tmp)
    if (tmp ~ /for[ \t(].* in .*([Cc]ases|CASES|[Tt]able|[Vv]ectors)/ || tmp ~ /([Cc]ases|CASES|[Tt]able)\.forEach/ || tmp ~ /@Test\(.*arguments:/) table = 1
    if (raw ~ /Thread\.sleep|Task\.sleep|URLSession\.shared|homeDirectoryForCurrentUser|NSHomeDirectory|Process\(\)/ && raw !~ /^[ \t]*\/\//) report(NR, "real-infra", "test touches real infrastructure: use an in-memory fake")
    next
  }
  if (raw ~ /^[ \t]*\/\/\//) { if (!pend) doc_line = NR; pend = 1; next }
  if (raw ~ /^[ \t]*@/ && raw !~ /(func|var|let|struct|class|enum) /) next
  isdecl = raw ~ /^[ \t]*((public|open|internal|private|fileprivate|final|static|override|mutating|indirect) +)*(func|struct|class|enum|actor|protocol|typealias|init|var|let|subscript)[ (]/
  ispublic = raw ~ /^[ \t]*((final|static|override|mutating|indirect) +)*(public|open) /
  if (pend && isdecl && !ispublic) report(doc_line, "doc-private", "doc comment on a non-public declaration")
  pend = 0
  s = strip(raw)
  i = index(s, "//")
  code = s
  if (i) {
    code = substr(s, 1, i - 1)
    text = substr(s, i + 2); sub(/^[ \t]+/, "", text)
    if (text ~ /^(SPEC|PLATFORM|SAFETY|DEPRECATED):/) check_tag(text, NR)
    else if (text !~ /^swiftlint:disable.* - /) report(NR, "comment", "inline comment (only SPEC:/PLATFORM:/DEPRECATED: allowed)")
  }
  if (s ~ /\/\*/) report(NR, "comment", "block comment")
  if (code ~ /try!|[ )]as!|fatalError\(|preconditionFailure\(|[^a-zA-Z_]print\(|[^a-zA-Z_]dump\(/) report(NR, "panic", "try!/as!/fatalError/preconditionFailure/print")
  if (code ~ /^(private |fileprivate |internal |public )?var / || code ~ /static var / || code ~ /static let shared/) report(NR, "global", "global mutable state or singleton")
  if (code ~ /func [A-Za-z_0-9]+(<[^>]*>)?\([^)]*: *Bool[ ,)=]/) report(NR, "bool-param", "Bool flag parameter; use an enum or two functions")
  if (code ~ /^(((public|open|internal|private|fileprivate|final|indirect) +)*)(struct|class|enum|actor|protocol) +[A-Za-z_]/) {
    t = code
    sub(/^((public|open|internal|private|fileprivate|final|indirect) +)*(struct|class|enum|actor|protocol) +/, "", t)
    match(t, /^[A-Za-z0-9_]+/)
    n++; names[n] = substr(t, 1, RLENGTH)
  }
}
END {
  if (mode == "test" && ntests > 2 && !table) report(1, "table-driven", ntests " test funcs and no cases table: one table, one behaviour per case")
  if (mode == "src") {
    if (n > 1) report(1, "multi-type", n " top-level types in one file: " names[1] ", " names[2] (n > 2 ? ", ..." : ""))
    if (n == 1 && names[1] != stem) report(1, "type-name", "type " names[1] " must live in " names[1] ".swift")
  }
  exit bad ? 1 : 0
}
