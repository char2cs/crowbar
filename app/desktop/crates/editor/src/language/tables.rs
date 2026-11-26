use super::known::Known;

pub(super) const PLAIN_TEXT: &str = "text";
pub(super) const ANGULAR_TEMPLATE: &str = "angular";
pub(super) const DOTENV: &str = "dotenv";
pub(super) const GIT_IGNORE: &str = "gitignore";
pub(super) const GIT_ATTRIBUTES: &str = "gitattributes";

pub(super) const ANGULAR_SUFFIXES: [&str; 2] = [".component.html", ".ng.html"];
pub(super) const GIT_INFO_EXCLUDE: &str = "/.git/info/exclude";
pub(super) const GIT_INFO_ATTRIBUTES: &str = "/.git/info/attributes";
pub(super) const ENV_FILE: &str = ".env";
const DOCKERFILE: &str = "dockerfile";

pub(super) const LANGUAGES: [Known; 61] = [
    Known {
        id: ANGULAR_TEMPLATE,
        name: "Angular Template",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "javascript",
        name: "JavaScript",
        extensions: &["js", "mjs", "cjs"],
        file_names: &[],
    },
    Known {
        id: "javascriptreact",
        name: "JSX",
        extensions: &["jsx"],
        file_names: &[],
    },
    Known {
        id: "typescript",
        name: "TypeScript",
        extensions: &["ts", "mts", "cts"],
        file_names: &[],
    },
    Known {
        id: "typescriptreact",
        name: "TSX",
        extensions: &["tsx"],
        file_names: &[],
    },
    Known {
        id: "python",
        name: "Python",
        extensions: &["py"],
        file_names: &[],
    },
    Known {
        id: "rust",
        name: "Rust",
        extensions: &["rs"],
        file_names: &[],
    },
    Known {
        id: "go",
        name: "Go",
        extensions: &["go"],
        file_names: &["go.mod", "go.sum", "go.work"],
    },
    Known {
        id: "java",
        name: "Java",
        extensions: &["java"],
        file_names: &[],
    },
    Known {
        id: "c",
        name: "C",
        extensions: &["c", "h"],
        file_names: &[],
    },
    Known {
        id: "cpp",
        name: "C++",
        extensions: &["cpp", "cc", "cxx", "hpp", "hh", "hxx"],
        file_names: &[],
    },
    Known {
        id: "csharp",
        name: "C#",
        extensions: &["cs"],
        file_names: &[],
    },
    Known {
        id: "ruby",
        name: "Ruby",
        extensions: &["rb"],
        file_names: &[],
    },
    Known {
        id: "php",
        name: "PHP",
        extensions: &["php"],
        file_names: &[],
    },
    Known {
        id: "html",
        name: "HTML",
        extensions: &["html", "htm"],
        file_names: &[],
    },
    Known {
        id: "css",
        name: "CSS",
        extensions: &["css"],
        file_names: &[],
    },
    Known {
        id: "diff",
        name: "Diff",
        extensions: &["diff", "patch"],
        file_names: &[],
    },
    Known {
        id: DOTENV,
        name: "Dotenv",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "json",
        name: "JSON",
        extensions: &["json", "jsonc"],
        file_names: &[],
    },
    Known {
        id: "yaml",
        name: "YAML",
        extensions: &["yaml", "yml"],
        file_names: &[],
    },
    Known {
        id: "toml",
        name: "TOML",
        extensions: &["toml"],
        file_names: &[],
    },
    Known {
        id: "markdown",
        name: "Markdown",
        extensions: &["md", "mdx", "markdown"],
        file_names: &[],
    },
    Known {
        id: "bash",
        name: "Bash",
        extensions: &["sh", "bash", "zsh"],
        file_names: &[".bashrc", ".zshrc", ".bash_profile", ".profile"],
    },
    Known {
        id: "swift",
        name: "Swift",
        extensions: &["swift"],
        file_names: &[],
    },
    Known {
        id: "kotlin",
        name: "Kotlin",
        extensions: &["kt", "kts"],
        file_names: &[],
    },
    Known {
        id: "scala",
        name: "Scala",
        extensions: &["scala"],
        file_names: &[],
    },
    Known {
        id: "lua",
        name: "Lua",
        extensions: &["lua"],
        file_names: &[],
    },
    Known {
        id: "nix",
        name: "Nix",
        extensions: &["nix"],
        file_names: &[],
    },
    Known {
        id: "dart",
        name: "Dart",
        extensions: &["dart"],
        file_names: &[],
    },
    Known {
        id: "elixir",
        name: "Elixir",
        extensions: &["ex", "exs"],
        file_names: &[],
    },
    Known {
        id: "ocaml",
        name: "OCaml",
        extensions: &["ml", "mli"],
        file_names: &[],
    },
    Known {
        id: "sql",
        name: "SQL",
        extensions: &["sql"],
        file_names: &[],
    },
    Known {
        id: "solidity",
        name: "Solidity",
        extensions: &["sol"],
        file_names: &[],
    },
    Known {
        id: "zig",
        name: "Zig",
        extensions: &["zig"],
        file_names: &[],
    },
    Known {
        id: "vue",
        name: "Vue",
        extensions: &["vue"],
        file_names: &[],
    },
    Known {
        id: "svelte",
        name: "Svelte",
        extensions: &["svelte"],
        file_names: &[],
    },
    Known {
        id: "embedded_template",
        name: "ERB",
        extensions: &["erb"],
        file_names: &[],
    },
    Known {
        id: PLAIN_TEXT,
        name: "Plain Text",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: DOCKERFILE,
        name: "Dockerfile",
        extensions: &[DOCKERFILE],
        file_names: &["containerfile", DOCKERFILE],
    },
    Known {
        id: GIT_ATTRIBUTES,
        name: "Git Attributes",
        extensions: &["gitattributes"],
        file_names: &[".gitattributes"],
    },
    Known {
        id: "graphql",
        name: "GraphQL",
        extensions: &["graphql", "gql"],
        file_names: &[],
    },
    Known {
        id: GIT_IGNORE,
        name: "Git Ignore",
        extensions: &["dockerignore", "gitignore", "ignore"],
        file_names: &[
            ".dockerignore",
            ".eslintignore",
            ".fdignore",
            ".gitignore",
            ".ignore",
            ".npmignore",
            ".prettierignore",
            ".rgignore",
            ".stylelintignore",
            ".vscodeignore",
        ],
    },
    Known {
        id: "scss",
        name: "SCSS",
        extensions: &["scss"],
        file_names: &[],
    },
    Known {
        id: "sass",
        name: "Sass",
        extensions: &["sass"],
        file_names: &[],
    },
    Known {
        id: "less",
        name: "Less",
        extensions: &["less"],
        file_names: &[],
    },
    Known {
        id: "xml",
        name: "XML",
        extensions: &[
            "xml", "xsl", "xslt", "xsd", "dtd", "wsdl", "svg", "plist", "csproj", "vbproj",
            "fsproj", "props", "targets",
        ],
        file_names: &[],
    },
    Known {
        id: "restructuredtext",
        name: "reStructuredText",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "fsharp",
        name: "F#",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "clojure",
        name: "Clojure",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "scheme",
        name: "Scheme",
        extensions: &["scm"],
        file_names: &[],
    },
    Known {
        id: "shell",
        name: "Shell",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "powershell",
        name: "PowerShell",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "batch",
        name: "Batch",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "ini",
        name: "INI",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "protobuf",
        name: "Protocol Buffers",
        extensions: &["proto"],
        file_names: &[],
    },
    Known {
        id: "ql",
        name: "QL",
        extensions: &["ql", "qll"],
        file_names: &[],
    },
    Known {
        id: "r",
        name: "R",
        extensions: &[],
        file_names: &[],
    },
    Known {
        id: "terraform",
        name: "Terraform",
        extensions: &["tf", "tfvars"],
        file_names: &[],
    },
    Known {
        id: "elm",
        name: "Elm",
        extensions: &["elm"],
        file_names: &[],
    },
    Known {
        id: "elisp",
        name: "Emacs Lisp",
        extensions: &["el"],
        file_names: &[],
    },
    Known {
        id: "lockfile",
        name: "Lockfile",
        extensions: &["lock"],
        file_names: &[],
    },
];
