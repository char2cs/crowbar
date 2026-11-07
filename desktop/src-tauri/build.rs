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
    tauri_build::try_build(tauri_build::Attributes::new().capabilities_path_pattern(capabilities))
        .expect("failed to run tauri-build");
}
