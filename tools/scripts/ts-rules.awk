# Per-file TypeScript rule scanner used by check-ts-rules.sh.
# vars: file (display path), mode (src|test)
function report(line, tag, msg) { printf "%s:%d: [%s] %s\n", file, line, tag, msg; bad = 1 }
function flush_comments() { }
function kebab(n,   o, i, c) {
  o = ""
  for (i = 1; i <= length(n); i++) { c = substr(n, i, 1); if (c ~ /[A-Z]/) o = o (i > 1 ? "-" : "") tolower(c); else o = o c }
  return o
}
function strip(s) {
  gsub(/\\./, "", s)
  gsub(/"[^"]*"/, "\"\"", s)
  gsub(/'[^']*'/, "''", s)
  gsub(/`[^`]*`/, "``", s)
  return s
}
BEGIN { pend_doc = 0; types = 0; run_len = 0; mocks = 0; comps = 0; classes = 0 }
{
  raw = $0
  if (raw ~ /^[ \t]*\/\//) {
    text = raw; sub(/^[ \t]*\/\/[ \t]*/, "", text)
    if (text ~ /^(SPEC|PLATFORM|SAFETY|DEPRECATED):/) check_tag(text, NR)
    else if (text !~ /^(eslint-|@ts-|prettier-ignore|biome-)/ && text !~ /^[#!]?(region|endregion)/)
      report(NR, "comment", "comment: only SPEC:/PLATFORM:/DEPRECATED: allowed (manifest 7); delete it or rename for intent")
    if (text ~ /(TODO|FIXME|XXX|HACK)/ && text !~ /(#[0-9]+|https?:\/\/)/) report(NR, "todo", "TODO/FIXME without an issue reference (#123 or URL)")
    if (raw ~ /eslint-disable|ts-ignore|ts-nocheck|ts-expect-error/) check_directive(raw)
    next
  }
  if (raw ~ /^[ \t]*\/\*\*/ || raw ~ /^[ \t]*\*/) { if (raw ~ /\*\/[ \t]*$/) { pend_doc = 1; doc_line = NR }; next }
  if (mode == "src" && raw !~ /^[ \t]*$/) {
    exported = raw ~ /^export (default )?(async )?(function|const|class|interface|type|enum|abstract)/
    declared = raw ~ /^(async )?(function|class|interface|type|enum|abstract class)[ \t]/ || raw ~ /^const [A-Za-z_]/
    if (!exported && declared && pend_doc) report(doc_line, "doc-private", "doc comment on a non-exported symbol")
    pend_doc = 0
  }
  if (raw ~ /^[ \t]*\/\*/ && raw !~ /^[ \t]*\/\*\*/ && raw !~ /eslint-|@ts-/) report(NR, "comment", "block comment: use // lines, max 3, explaining why")
  if (raw ~ /eslint-disable|@ts-ignore|@ts-nocheck|@ts-expect-error/) check_directive(raw)
  s = strip(raw)
  code = s
  if (match(s, /[ \t]\/\/.*/)) {
    tail = substr(s, RSTART)
    code = substr(s, 1, RSTART - 1)
    if (tail ~ /(TODO|FIXME|XXX|HACK)/ && tail !~ /(#[0-9]+|https?:\/\/)/) report(NR, "todo", "TODO/FIXME without an issue reference")
  }
  if (code ~ /(:|as|<|=) *any([^A-Za-z0-9_]|$)/ || code ~ /Array<any>|any\[\]/) report(NR, "any", "explicit any: use unknown or a real type")
  if (mode == "src" && code ~ /[A-Za-z0-9_)\]]!(\.|\[|\)|;|,|$)/ && code !~ /!==|!=/) report(NR, "non-null", "non-null assertion (!): narrow with a guard instead")
  if (code ~ /export \* from/) report(NR, "barrel", "export * barrel: import from the defining module")
  if (mode == "src" && code ~ /^(let|var) /) report(NR, "global", "module-level mutable binding (let/var): pass state in")
  if (code ~ /^(export )?const [A-Za-z0-9_]+ *(:[^=]+)?= *new (Map|Set|WeakMap|WeakSet)\(/ && mode == "src") report(NR, "global", "module-level mutable collection: own it in a class/store passed in")
  if (code ~ /(globalThis|window)\.[A-Za-z_]+ *=[^=]/) report(NR, "global", "assigning to globalThis/window")
  if (code ~ /console\.(log|debug|info)\(/ && mode == "src") report(NR, "console", "console output in source: use the logging/report path")
  if (mode == "src") {
    if (code ~ /^export (default )?(async )?function [A-Z][A-Za-z0-9]*/ || code ~ /^export const [A-Z][A-Za-z0-9]* *(:[^=]+)?= *(\(|memo|forwardRef|React\.)/) comps++
    if (code ~ /^export (default )?(abstract )?class /) classes++
    if (code ~ /^export (default )?(abstract )?(class|interface|enum) [A-Z]/ || code ~ /^export type [A-Z][A-Za-z0-9]* *(<[^=]*>)? *=/) {
      t = code; sub(/^export (default )?(abstract )?(class|interface|enum|type) +/, "", t); match(t, /^[A-Za-z0-9]+/); types++; tname[types] = substr(t, 1, RLENGTH)
    }
    if (code ~ /^export (default )?(async )?function [A-Z]/) { t = code; sub(/^export (default )?(async )?function +/, "", t); match(t, /^[A-Za-z0-9]+/); types++; tname[types] = substr(t, 1, RLENGTH) }
  }
  if (mode == "test") {
    if (code ~ /vi\.mock\(/) mocks++
    if (code ~ /(^|[^A-Za-z_.])(it|test)\(/) cases++
    if (code ~ /\.each\(/) table = 1
    if (code ~ /process\.env\.HOME|homedir\(\)|child_process|new WebSocket\(|https?:\/\/[a-z0-9-]+\.(com|io|dev|org)/) report(NR, "real-infra", "test touches real infrastructure: use an in-memory fake")
    if (code ~ /toHaveClass\(|readFileSync\(/ ) report(NR, "test-style", "tests assert behaviour, not class names or src text")
    if (code ~ /setTimeout\(/ && code ~ /Promise/) report(NR, "sleep", "real sleep in a test: use fake timers, waitFor or events")
    if (code ~ /\.(skip|only)\(|\b(xit|xdescribe|fit|fdescribe)\(/) report(NR, "test-skip", "skipped/focused test: a known bug is an issue, not a skip")
  }
}
function check_directive(l) {
  if (l ~ /@ts-ignore|@ts-nocheck/) report(NR, "suppress", "@ts-ignore/@ts-nocheck banned; use @ts-expect-error with a reason")
  else if (l ~ /@ts-expect-error/ && l !~ /@ts-expect-error[ \t]*[-:]?[ \t]*[A-Za-z]{3,}/) report(NR, "suppress", "@ts-expect-error needs a reason on the same line")
  if (l ~ /eslint-disable/ && l !~ /eslint-disable[^-]*(-next-line|-line)?[ \t]+[@A-Za-z\/-]+.* -- [A-Za-z]/) report(NR, "suppress", "eslint-disable needs the rule name and ' -- reason' on the same line")
}
END {
  flush_comments()
  if (mode == "test" && cases > 2 && !table) report(1, "table-driven", cases " it() blocks and no it.each table: one table, one behaviour per case")
  if (mode == "test" && mocks > 3) report(1, "mock-count", mocks " vi.mock calls (max 3)")
  if (mode == "src" && types > 1) report(1, "multi-concept", types " exported types/components in one file: " tname[1] ", " tname[2])
  if (mode == "src" && types == 1 && kebab(tname[1]) != stem && stem != "index") report(1, "type-name", "type " tname[1] " must live in " kebab(tname[1]) ".ts(x)")
  if (mode == "src" && classes > 1) report(1, "multi-concept", classes " exported classes in one file")
  exit bad ? 1 : 0
}
