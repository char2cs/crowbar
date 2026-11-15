//! Registers the mobile capability set with the Tauri build.

use std::error::Error;

fn main() -> Result<(), Box<dyn Error>> {
    println!("cargo:rerun-if-changed=capabilities");
    let attributes =
        tauri_build::Attributes::new().capabilities_path_pattern("./capabilities/mobile/**/*");
    tauri_build::try_build(attributes)?;
    Ok(())
}
