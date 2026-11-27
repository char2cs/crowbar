# Structure rules shared by Rust, Swift and TypeScript (manifest rules 1-6, 11, 12, 30, 32, 33).
# vars: file, lang (rust|swift|ts), mode (src|test)
function report(line, tag, msg) { if (generated) return; printf "%s:%d: [%s] %s\n", file, line, tag, msg; bad = 1 }
function strip(s) {
  if (lang == "rust") gsub(/r#+"[^\n]*"#+/, "\"\"", s)
  gsub(/\\./, "", s)
  gsub(/"[^"]*"/, "\"\"", s)
  if (lang == "rust") gsub(/'[^']'/, "''", s); else gsub(/'[^']*'/, "''", s)
  if (lang == "ts") gsub(/`[^`]*`/, "``", s)
  return s
}
function in_trait_impl(   i) { for (i = sp; i >= 1; i--) if (kind[i] == "C") return trait_impl[i]; return 0 }
function count(s, re,   n) { n = 0; while (match(s, re)) { n++; s = substr(s, RSTART + RLENGTH) } return n }
function top() { return sp > 0 ? kind[sp] : "" }
function has_fn() { for (i = sp; i >= 1; i--) if (kind[i] == "F") return 1; return 0 }
function blevel(   i, n) { n = 0; for (i = sp; i >= 1 && kind[i] != "F"; i--) if (kind[i] == "B") n++; return n }
function is_fn_decl(t) {
  if (lang == "rust") return t ~ /(^|[^A-Za-z_])fn[ \t]+[A-Za-z_]/
  if (lang == "swift") return t ~ /(^|[^A-Za-z_.])(func[ \t]+[^ \t]|init[ \t]*[(<?])/
  return t ~ /(^|[^A-Za-z_])function[ \t*]/ || t ~ /=>[ \t]*$/
}
function classify(t) {
  if (macro_sp > 0 && sp >= macro_sp) return "C"
  if (t ~ /(^|[^A-Za-z_])(impl|trait|mod|struct|enum|union|class|interface|extension|protocol|namespace|actor|macro_rules!)([^A-Za-z_]|$)/ && !is_fn_decl(t)) return "C"
  if (lang == "ts" && t ~ /(describe|suite|context)(\.[a-z]+)?\(/) return "C"
  if (t ~ /(^|[^A-Za-z_])(match|switch)([^A-Za-z_]|$)/ && t !~ /=>/) return "M"
  if (has_fn()) return "B"
  if (is_fn_decl(t) || t ~ /=>|\)[ \t]*(:[^{=]+)?[ \t]*$/ || t ~ /\|[^|]*\|[ \t]*$/) return "F"
  if (lang == "swift" && t ~ /(var|let|get|set|init|subscript)/) return "F"
  return "C"
}
function rust_paren(code,   i, c, d) {
  if (!match(code, /(^|[^A-Za-z_])fn[ \t]+[A-Za-z_0-9]+/)) return 0
  i = RSTART + RLENGTH
  while (i <= length(code)) {
    c = substr(code, i, 1)
    if (c == "(") return i
    if (c != "<") { i++; continue }
    d = 1; i++
    while (i <= length(code) && d > 0) {
      c = substr(code, i, 1)
      if (c == "<") d++
      else if (c == ">" && substr(code, i - 1, 1) != "-") d--
      i++
    }
  }
  return 0
}
function type_colon(p,   i, c, d) {
  d = 0
  for (i = 1; i <= length(p); i++) {
    c = substr(p, i, 1)
    if (c ~ /[(\[{<]/) d++
    else if (c ~ /[)\]}>]/) d--
    else if (c == ":" && d == 0 && substr(p, i + 1, 1) != ":" && substr(p, i - 1, 1) != ":") return i
  }
  return 0
}
function bound_names(pat,   n, i, t, w, c) {
  gsub(/[A-Za-z_0-9]+::/, " ", pat)
  gsub(/[^A-Za-z_0-9]/, " ", pat)
  n = split(pat, w, / +/); c = 0
  for (i = 1; i <= n; i++) if (w[i] ~ /^[a-z_][a-z0-9_]*$/ && w[i] != "mut" && w[i] != "ref" && w[i] != "_") c++
  return c < 1 ? 1 : c
}
function param_weight(cur,   pos, pat) {
  if (cur !~ /[A-Za-z]/) return 0
  if (cur ~ /^[ \t&]*('[A-Za-z_]+[ \t]+)?(mut[ \t]+)?(self|_ *self)[ \t]*(:.*)?$/) return 0
  if (cur ~ /^[ \t]*(file|line)[ \t]*:[ \t]*(StaticString|UInt)/) return 0
  if (lang != "rust") return 1
  pos = type_colon(cur); pat = pos ? substr(cur, 1, pos - 1) : cur
  return pat ~ /[(\[{]/ ? bound_names(pat) : 1
}
function has_bool(text) {
  if (lang == "rust") return text ~ /(^|[^A-Za-z_0-9])bool([^A-Za-z_0-9]|$)/
  if (lang == "swift") return text ~ /(^|[^A-Za-z_0-9])Bool([^A-Za-z_0-9]|$)/
  return text ~ /:[ \t]*boolean[ \t]*[,)]/
}
function check_params(text, start, last,   n, i, c, d, cur, np, nraw, plist) {
  gsub(/->[ \t]*bool([^A-Za-z_0-9]|$)/, "->", text)
  gsub(/->|=>/, "", text)
  d = 0; np = 0; nraw = 0; cur = ""
  for (i = 1; i <= length(text); i++) {
    c = substr(text, i, 1)
    if (c ~ /[(\[{<]/) d++
    else if (c ~ /[)\]}>]/) d--
    if (d == 0 && i > 1) { plist = substr(text, 1, i); break }
    if (d == 1 && c == ",") { np += param_weight(cur); nraw += param_weight(cur) ? 1 : 0; cur = ""; continue }
    if (i != 1) cur = cur c
  }
  np += param_weight(cur); nraw += param_weight(cur) ? 1 : 0
  if (plist == "") plist = text
  if (np > 4 && !in_trait_impl()) report(start, "param-count", np " parameters (max 4; a tuple or destructuring pattern counts each bound name): pass a struct/options object")
  if (!(lang == "rust" && in_trait_impl()) && has_bool(plist)) report(start, "bool-param", "boolean flag parameter: use an enum or two functions")
  if (nraw >= 2 && start == last && lang != "ts") report(start, "param-layout", "one parameter per line, '(' on the signature line and ')' on its own line")
}
function is_const_def(code) {
  if (lang == "rust") return code ~ /^[ \t]*(pub(\([^)]*\))?[ \t]+)?(const|static)[ \t]+(mut[ \t]+)?[A-Z_][A-Z_0-9]*[ \t]*:/
  if (lang == "swift") return code ~ /(static|class)[ \t]+(let|var)[ \t]/ || code ~ /^((public|internal|private|fileprivate)[ \t]+)?let[ \t]/
  return code ~ /^(export[ \t]+)?const[ \t]/
}
function lit_ok(lit) { return lit ~ /^(-?0|-?1|2|2\.0|0\.0|1\.0|-?0\.5|10|100|0[xXbBoO]_*[01]_*)$/ }
function magic_numbers(code,   t, lit, prev) {
  t = code
  while (match(t, /0[xXbBoO][0-9A-Fa-f_]+/)) {
    lit = substr(t, RSTART, RLENGTH); prev = RSTART > 1 ? substr(t, RSTART - 1, 1) : " "
    if (prev !~ /[A-Za-z_0-9]/ && !lit_ok(lit)) report(NR, "magic-number", "magic number " lit ": name it with a const")
    t = substr(t, 1, RSTART - 1) " " substr(t, RSTART + RLENGTH)
  }
  while (match(t, /[0-9]_+[0-9]/)) t = substr(t, 1, RSTART) substr(t, RSTART + RLENGTH - 1)
  while (match(t, /(^|[^A-Za-z_0-9.])[0-9][0-9.]*[eE][-+]?[0-9]+/)) {
    lit = substr(t, RSTART, RLENGTH); gsub(/^[^0-9]/, "", lit)
    report(NR, "magic-number", "magic number " lit ": name it with a const")
    t = substr(t, 1, RSTART - 1) " " substr(t, RSTART + RLENGTH)
  }
  gsub(/[A-Za-z_][A-Za-z_0-9]*/, " ", t); gsub(/\.\./, " ", t)
  while (match(t, /(^|[^0-9A-Za-z_.])-?[0-9]+(\.[0-9]+)?/)) {
    lit = substr(t, RSTART, RLENGTH); gsub(/[^0-9.-]/, "", lit)
    if (!lit_ok(lit)) report(NR, "magic-number", "magic number " lit ": name it with a const")
    t = substr(t, RSTART + RLENGTH)
  }
}
function collect_strings(r,   lit, re) {
  re = (lang == "ts") ? "\"([^\"\\\\]|\\\\.)+\"|'([^'\\\\]|\\\\.)+'" : "\"([^\"\\\\]|\\\\.)+\""
  while (match(r, re)) {
    lit = substr(r, RSTART, RLENGTH); r = substr(r, RSTART + RLENGTH)
    if (length(lit) < 5 || lit ~ /^["'][^A-Za-z0-9]*["']$/) continue
    if (!(lit in str_count)) str_first[lit] = NR
    str_count[lit]++
  }
}
function track_accumulator(code,   nm, rhs, tail) {
  if (is_fn_decl(code)) delete acc_line
  if (match(code, /(^|[^A-Za-z_])(let mut|var|let)[ \t]+[a-z_][A-Za-z_0-9]*[ \t]*(:[^=]*)?=([^=]|$)/) || (lang == "ts" && match(code, /(^|[^A-Za-z_])const[ \t]+[a-z_][A-Za-z_0-9]*[ \t]*(:[^=]*)?=([^=]|$)/))) {
    rhs = substr(code, RSTART + RLENGTH - 1)
    nm = substr(code, RSTART, RLENGTH); sub(/^.*(let mut|var|let|const)[ \t]+/, "", nm); match(nm, /^[a-z_][A-Za-z_0-9]*/); nm = substr(nm, 1, RLENGTH)
    if (rhs ~ /^[ \t]*(Vec|String|HashMap|HashSet|BTreeMap|BTreeSet|VecDeque)::(new|with_capacity)\(.*\)[ \t]*;?[ \t]*$/ || rhs ~ /^[ \t]*(vec!\[\]|Default::default\(\)|0|0\.0|0usize|0u[0-9]+|""|''|\[\]|\{\}|\[[A-Za-z:<>?]*\]\(\)|String\(\)|new (Map|Set)\(\))[ \t]*;?[ \t]*$/) acc_line[nm] = NR
    return
  }
  if (code !~ /^[ \t]*(return[ \t]+)?((Ok|Some)\()?[a-z_][A-Za-z_0-9]*\)?[ \t]*;?[ \t]*$/) return
  tail = code; sub(/^[ \t]*(return[ \t]+)?((Ok|Some)\()?/, "", tail); sub(/\)?[ \t]*;?[ \t]*$/, "", tail)
  if (tail in acc_line) { report(acc_line[tail], "named-return", "mutable accumulator `" tail "` returned at the end: return a named expression/type"); delete acc_line[tail] }
}
function decl_start(code) {
  if (lang == "rust") return code ~ /(^|[^A-Za-z_])fn[ \t]+[A-Za-z_0-9]+/
  if (lang == "swift") return code ~ /(^|[^A-Za-z_.])(func[ \t]+[^ \t(<]+|init)[ \t]*[(<?]/
  if (code ~ /(^|[^A-Za-z_])function[ \t*]+[A-Za-z_$0-9]*[ \t]*[(<]/) return 1
  if (code ~ /(const|let) [A-Za-z_$0-9]+[ \t]*(:[^=]+)?=[ \t]*(async[ \t]+)?\(/) return 1
  if (code ~ /^[ \t]*(if|for|while|switch|catch|return|await|expect|it|describe|test)[ \t(]/) return 0
  if (code ~ /=>[ \t]*[^{ ]/) return 0
  return code ~ /^[ \t]+(public |private |protected |static |async |readonly )*[A-Za-z_]+\(/ && code ~ /\)[^;]*$/
}
BEGIN { macro_sp = 0; sp = 0; carry = ""; decl_active = 0 }
{
  if (FNR == 1) generated = ($0 ~ /generated by .*Do not edit/)
  raw = $0
  if (lang == "swift") {
    n3 = count(raw, "\"\"\"")
    if (inml) {
      if (n3 % 2 == 0) next
      inml = 0; raw = substr(raw, index(raw, "\"\"\"") + 3)
    } else if (n3 % 2 == 1) {
      inml = 1; raw = substr(raw, 1, index(raw, "\"\"\"") - 1) "\"\""
    }
  }
  if (inblock) { if (raw ~ /\*\//) inblock = 0; next }
  if (raw ~ /^[ \t]*\/\*/ && raw !~ /\*\//) { inblock = 1; next }
  s = strip(raw)
  code = s
  if (match(s, /\/\/.*/)) code = substr(s, 1, RSTART - 1)
  if (code ~ /^[ \t]*$/) next
  rawcode = code

  tests = (mode == "test") || (nt > 0)

  if (decl_active) {
    decl_text = decl_text " " code
    if (count(decl_text, "[(]") <= count(decl_text, "[)]")) {
      check_params(substr(decl_text, index(decl_text, "(")), decl_line, NR)
      decl_active = 0
    }
  } else if (decl_start(code)) {
    i = (lang == "rust") ? rust_paren(code) : index(code, "(")
    if (i) {
      decl_active = 1; decl_line = NR; decl_text = substr(code, i)
      if (count(decl_text, "[(]") <= count(decl_text, "[)]")) {
        check_params(decl_text, NR, NR); decl_active = 0
      }
    }
  }

  if (!tests) {
    if (code ~ /(^|[^A-Za-z_])(let mut|var|let) (result|ret|res|out|output|acc|accum|retval)[ \t:=;]/ && code !~ /^[ \t]*(let|const) (result|value)[ \t]*=.*\)\?*;/ ) report(NR, "named-return", "mutable accumulator returned at the end: return a named expression/type")
    else track_accumulator(code)
    if (lang == "ts" && code ~ /for[ \t]*\((let|var) [a-z]+ = 0/) report(NR, "hand-loop", "index loop: use map/filter/find/for-of")
    if (lang == "swift" && code ~ /for [a-z]+ in 0\.\.</) report(NR, "hand-loop", "index loop: use map/filter/first(where:)/indices")
    if (lang == "rust" && code ~ /\|[^|]*:[ \t]*&?bool[^|]*\|/) report(NR, "bool-param", "boolean flag closure parameter: use an enum or two functions")
    if (code ~ /(const|static let|static final let)[^=]*=[^=]*[[{(][ \t]*$/) tabledepth = 1
    else if (tabledepth) tabledepth += count(code, "[[{(]") - count(code, "[]})]")
    if (!generated && tabledepth <= 0 && !is_const_def(code) && code !~ /^[ \t]*(@|#!?\[|enum)/ && code !~ /^[ \t]*case [A-Za-z_][A-Za-z_0-9]*[ \t]*=/ && code !~ /#available/) magic_numbers(code)
    if (!generated && !is_const_def(code) && raw !~ /^[ \t]*(\/\/|#!?\[|use |mod |import |export .* from|extern )/) collect_strings(raw)
  }
  if (code ~ /}[ \t]*else([^A-Za-z_]|$)/ || (lang != "rust" && code ~ /^[ \t]*else([^A-Za-z_]|$)/ && prevcode ~ /}[ \t]*$/) || (lang == "ts" && code ~ /^[ \t]*else/))
    report(NR, "else", "no else: use a guard clause and return early")
  prevcode = code
  if (code ~ /(^|[^A-Za-z_])(if|while|guard|unless|else if)[ \t(]/ && count(code, "&&|[|][|]") >= 2)
    report(NR, "long-condition", "compound condition: bind it to a named variable that reads like a sentence")
  if (lang == "rust" && code ~ /^(pub(\([^)]*\))? )?struct[ \t]+[A-Za-z_0-9]+[ \t]*(;|\{[ \t]*\})/) report(NR, "stateless-type", "stateless struct: use a free function in a module")
  if (lang == "swift" && code ~ /^[ \t]*init\(\)/) report(NR, "new-stateless", "init() with no parameters: only when state needs initialising")
  if (lang == "rust" && code ~ /fn new\(\)/) report(NR, "new-stateless", "new() with no parameters: only when state needs initialising")
  if (lang == "ts" && code ~ /constructor\(\)/) report(NR, "new-stateless", "empty constructor")

  buf = carry
  if (lang == "swift" && code ~ /^[ \t]*((final|public|internal|private|fileprivate|open)[ \t]+)*(class|struct|enum|extension|protocol|actor)[ \t]/) buf = ""
  for (j = 1; j <= length(code); j++) {
    c = substr(code, j, 1)
    if (c == "{") {
      k = classify(buf)
      if (lang == "rust" && buf ~ /macro_rules!/ && macro_sp == 0) macro_sp = sp + 1
      sp++; kind[sp] = k; trait_impl[sp] = (k == "C" && buf ~ /(^|[^A-Za-z_])impl[^{]* for /) ? 1 : 0; openline[sp] = NR; lvl[sp] = (k == "B") ? blevel() : 0
      if (k == "C" && buf ~ /(^|[^A-Za-z_])mod[ \t]+tests[ \t]*$/ && lang == "rust") { nt++; testframe[sp] = 1 } else testframe[sp] = 0
      buf = ""
    } else if (c == "}") {
      if (sp > 0) {
        if (testframe[sp]) nt--
        if (kind[sp] == "F" && NR - openline[sp] + 1 > 40) report(openline[sp], "fn-length", "function body is " (NR - openline[sp] + 1) " lines (max 40): extract a function")
        if (kind[sp] == "B" && lvl[sp] >= 2 && NR != openline[sp]) report(openline[sp], "depth", "nesting level 3 inside a function (max 2): extract a function")
        if (macro_sp > 0 && sp == macro_sp) macro_sp = 0
        sp--
      }
      buf = ""
    } else if (c == ";") buf = ""
    else buf = buf c
  }
  carry = buf " "
  if (sp == 0 && code ~ /;[ \t]*$/) carry = ""
}
END {
  for (lit in str_count) if (str_count[lit] >= 3) report(str_first[lit], "magic-string", "string literal " lit " appears " str_count[lit] " times: name it with a const")
  exit bad ? 1 : 0
}
