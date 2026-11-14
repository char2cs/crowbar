#!/usr/bin/env bash
# Reference app only: dump getComputedStyle + geometry for the shell's key elements to JSON.
# Usage: dump-styles.sh <selectors.txt> <out.json>   (one "label<TAB>css selector" per line)
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
export PARITY_CONTAINER=${PARITY_CONTAINER:-inventory-app} PARITY_OUT=${PARITY_OUT:-/tmp/parity-dump}
mkdir -p "$PARITY_OUT"; source "$here/lib.sh"
sel=$(awk -F'\t' 'NF==2{printf "%s%s", sep, "[\"" $1 "\",\"" $2 "\"]"; sep=","}' "$1")
props='["x","y","width","height","display","position","backgroundColor","color","opacity","fontFamily","fontSize","fontWeight","lineHeight","letterSpacing","paddingTop","paddingRight","paddingBottom","paddingLeft","marginTop","marginRight","marginBottom","marginLeft","borderTopWidth","borderTopColor","borderTopLeftRadius","boxShadow","gap","zIndex","transitionProperty","transitionDuration","transitionTimingFunction","animationName","animationDuration","backdropFilter","overflow","cursor","flexGrow","flexShrink","flexBasis"]'
js 'var out={};var S=['"$sel"'];var P='"$props"';S.forEach(function(s){var els=document.querySelectorAll(s[1]);out[s[0]]=[].slice.call(els,0,3).map(function(e){var cs=getComputedStyle(e),r=e.getBoundingClientRect(),o={sel:s[1],text:(e.textContent||"").trim().slice(0,30)};P.forEach(function(p){o[p]=(p=="x"?r.x:p=="y"?r.y:p=="width"?r.width:p=="height"?r.height:cs[p])});return o})});return JSON.stringify(out)' > "$2"
