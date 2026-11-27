pub(super) struct Known(
    pub(super) &'static str,
    pub(super) &'static str,
    pub(super) &'static [&'static str],
    pub(super) &'static [&'static str],
);

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
    Known(ANGULAR_TEMPLATE, "Angular Template", &[], &[]),
    Known("javascript", "JavaScript", &["js", "mjs", "cjs"], &[]),
    Known("javascriptreact", "JSX", &["jsx"], &[]),
    Known("typescript", "TypeScript", &["ts", "mts", "cts"], &[]),
    Known("typescriptreact", "TSX", &["tsx"], &[]),
    Known("python", "Python", &["py"], &[]),
    Known("rust", "Rust", &["rs"], &[]),
    Known("go", "Go", &["go"], &["go.mod", "go.sum", "go.work"]),
    Known("java", "Java", &["java"], &[]),
    Known("c", "C", &["c", "h"], &[]),
    Known("cpp", "C++", &["cpp", "cc", "cxx", "hpp", "hh", "hxx"], &[]),
    Known("csharp", "C#", &["cs"], &[]),
    Known("ruby", "Ruby", &["rb"], &[]),
    Known("php", "PHP", &["php"], &[]),
    Known("html", "HTML", &["html", "htm"], &[]),
    Known("css", "CSS", &["css"], &[]),
    Known("diff", "Diff", &["diff", "patch"], &[]),
    Known(DOTENV, "Dotenv", &[], &[]),
    Known("json", "JSON", &["json", "jsonc"], &[]),
    Known("yaml", "YAML", &["yaml", "yml"], &[]),
    Known("toml", "TOML", &["toml"], &[]),
    Known("markdown", "Markdown", &["md", "mdx", "markdown"], &[]),
    Known(
        "bash",
        "Bash",
        &["sh", "bash", "zsh"],
        &[".bashrc", ".zshrc", ".bash_profile", ".profile"],
    ),
    Known("swift", "Swift", &["swift"], &[]),
    Known("kotlin", "Kotlin", &["kt", "kts"], &[]),
    Known("scala", "Scala", &["scala"], &[]),
    Known("lua", "Lua", &["lua"], &[]),
    Known("nix", "Nix", &["nix"], &[]),
    Known("dart", "Dart", &["dart"], &[]),
    Known("elixir", "Elixir", &["ex", "exs"], &[]),
    Known("ocaml", "OCaml", &["ml", "mli"], &[]),
    Known("sql", "SQL", &["sql"], &[]),
    Known("solidity", "Solidity", &["sol"], &[]),
    Known("zig", "Zig", &["zig"], &[]),
    Known("vue", "Vue", &["vue"], &[]),
    Known("svelte", "Svelte", &["svelte"], &[]),
    Known("embedded_template", "ERB", &["erb"], &[]),
    Known(PLAIN_TEXT, "Plain Text", &[], &[]),
    Known(
        DOCKERFILE,
        "Dockerfile",
        &[DOCKERFILE],
        &["containerfile", DOCKERFILE],
    ),
    Known(
        GIT_ATTRIBUTES,
        "Git Attributes",
        &["gitattributes"],
        &[".gitattributes"],
    ),
    Known("graphql", "GraphQL", &["graphql", "gql"], &[]),
    Known(
        GIT_IGNORE,
        "Git Ignore",
        &["dockerignore", "gitignore", "ignore"],
        &[
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
    ),
    Known("scss", "SCSS", &["scss"], &[]),
    Known("sass", "Sass", &["sass"], &[]),
    Known("less", "Less", &["less"], &[]),
    Known(
        "xml",
        "XML",
        &[
            "xml", "xsl", "xslt", "xsd", "dtd", "wsdl", "svg", "plist", "csproj", "vbproj",
            "fsproj", "props", "targets",
        ],
        &[],
    ),
    Known("restructuredtext", "reStructuredText", &[], &[]),
    Known("fsharp", "F#", &[], &[]),
    Known("clojure", "Clojure", &[], &[]),
    Known("scheme", "Scheme", &["scm"], &[]),
    Known("shell", "Shell", &[], &[]),
    Known("powershell", "PowerShell", &[], &[]),
    Known("batch", "Batch", &[], &[]),
    Known("ini", "INI", &[], &[]),
    Known("protobuf", "Protocol Buffers", &["proto"], &[]),
    Known("ql", "QL", &["ql", "qll"], &[]),
    Known("r", "R", &[], &[]),
    Known("terraform", "Terraform", &["tf", "tfvars"], &[]),
    Known("elm", "Elm", &["elm"], &[]),
    Known("elisp", "Emacs Lisp", &["el"], &[]),
    Known("lockfile", "Lockfile", &["lock"], &[]),
];
