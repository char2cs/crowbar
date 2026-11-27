mod query_patch;

pub use query_patch::QueryPatch;

pub const PATCHES: [QueryPatch; 2] = [
    QueryPatch {
        language: "markdown",
        hunks: &[
            (
                "(atx_heading (inline) @title)\n(setext_heading (paragraph) @title)\n",
                "(atx_heading) @title\n",
            ),
            (
                "  (atx_h1_marker)\n  (atx_h2_marker)\n  (atx_h3_marker)\n  (atx_h4_marker)\n  \
                 (atx_h5_marker)\n  (atx_h6_marker)\n  (setext_h1_underline)\n  \
                 (setext_h2_underline)\n] @punctuation.special",
                "  (setext_h1_underline)\n  (setext_h2_underline)\n] @title",
            ),
            (
                "[\n  (block_continuation)\n  (block_quote_marker)\n] @punctuation.special",
                "(block_continuation) @punctuation.special\n\n(block_quote_marker) @comment",
            ),
        ],
    },
    QueryPatch {
        language: "typescript",
        hunks: &[(
            "(predefined_type) @type.builtin",
            "(predefined_type) @keyword",
        )],
    },
];
