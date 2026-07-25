; --- from https://raw.githubusercontent.com/nvim-treesitter/nvim-treesitter/4916d6592ede8c07973490d9322f187e07dfefac/runtime/queries/json/highlights.scm ---
[
  (true)
  (false)
] @boolean

(null) @constant.builtin

(number) @number

(pair
  key: (string) @property)

(pair
  value: (string) @string)

(array
  (string) @string)

[
  ","
  ":"
] @punctuation.delimiter

[
  "["
  "]"
  "{"
  "}"
] @punctuation.bracket

("\"" @conceal
  )

(escape_sequence) @string.escape

((escape_sequence) @conceal
  (#eq? @conceal "\\\"")
  )

(comment) @comment @spell
