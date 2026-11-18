use std::io::{self, Write};
use std::path::Path;
use std::process::{Command, Stdio};

const WRITE_EXECUTABLE: &str = "cat > \"$1\" && chmod 755 \"$1\"";

/// Writes `body` to `path` as an executable script through a short-lived `/bin/sh` child, so no
/// descriptor of the script ever exists in the test process. A sibling test thread that forks
/// while the script is being written therefore cannot inherit a writer of it, and executing the
/// script never fails with `ETXTBSY` ("Text file busy").
#[ignore = "real-infra: runs a real shell that writes an executable file"]
pub(crate) fn write_script(
    path: &Path,
    body: &str,
) -> io::Result<()> {
    let mut writer = Command::new("/bin/sh")
        .args(["-c", WRITE_EXECUTABLE, "sh"])
        .arg(path)
        .stdin(Stdio::piped())
        .spawn()?;
    let mut input = writer
        .stdin
        .take()
        .ok_or_else(|| io::Error::other("no stdin pipe"))?;
    input.write_all(body.as_bytes())?;
    drop(input);
    let status = writer.wait()?;
    if !status.success() {
        return Err(io::Error::other(format!(
            "writing {} failed: {status}",
            path.display()
        )));
    }
    Ok(())
}
