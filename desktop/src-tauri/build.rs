fn main() {
    // The sidecar shell permissions only exist where tauri-plugin-shell is built.
    let mobile = matches!(
        std::env::var("CARGO_CFG_TARGET_OS").as_deref(),
        Ok("ios" | "android")
    );
    let capabilities = if mobile {
        "./capabilities/mobile/**/*"
    } else {
        "./capabilities/desktop/**/*"
    };
    println!("cargo:rerun-if-changed=capabilities");
    stamp_build();
    tauri_build::try_build(tauri_build::Attributes::new().capabilities_path_pattern(capabilities))
        .expect("failed to run tauri-build");
}

/// Names the commit this shell is built from, "dev" outside a checkout. The
/// shell adopts no daemon of another build, since its web bundle speaks only to
/// its own daemon.
fn stamp_build() {
    let git = |args: &[&str]| {
        std::process::Command::new("git")
            .args(args)
            .output()
            .ok()
            .filter(|out| out.status.success())
            .and_then(|out| String::from_utf8(out.stdout).ok())
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty())
    };
    let build = git(&["rev-parse", "HEAD"]).unwrap_or_else(|| "dev".into());
    println!("cargo:rustc-env=CROWBAR_BUILD={build}");
    for path in ["HEAD", "logs/HEAD"] {
        if let Some(path) = git(&["rev-parse", "--git-path", path]) {
            println!("cargo:rerun-if-changed={path}");
        }
    }
}
