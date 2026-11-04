// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "tauri-plugin-crowbar-chrome",
    platforms: [.iOS("26.0")],
    products: [
        .library(name: "tauri-plugin-crowbar-chrome", type: .static, targets: ["tauri-plugin-crowbar-chrome"]),
    ],
    dependencies: [
        .package(name: "Tauri", path: "../.tauri/tauri-api"),
    ],
    targets: [
        .target(
            name: "tauri-plugin-crowbar-chrome",
            dependencies: [.byName(name: "Tauri")],
            path: "Sources"),
        .testTarget(
            name: "ChromePluginTests",
            dependencies: ["tauri-plugin-crowbar-chrome"],
            path: "Tests/ChromePluginTests",
            // AppScheme.swift declares Rust-provided externs; the host app links them.
            linkerSettings: [.unsafeFlags(["-Xlinker", "-undefined", "-Xlinker", "dynamic_lookup"])]),
    ]
)
