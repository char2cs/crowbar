//! A page kept hidden, as the shell keeps its spare pages, paints over the page it replaces once
//! it is shown.

use std::time::Instant;

use webview::Visibility;
use x11rb::rust_connection::RustConnection;

use super::pixels::{Region, shows_before};
use super::{KEY_WAIT, PAGE_WAIT, SIDE, TestResult, page_in, wait_for};

const DARK_WHILE_HIDDEN: &str = "document.documentElement.style.background = 'black';
window.ipc.postMessage('dark')";
const WHOLE: Region = (0, 0, SIDE, SIDE);

pub(super) fn a_spare_page_paints_over_the_page_it_replaces(
    connection: &RustConnection,
    host: &webview::Host,
    parent: u32,
) -> TestResult {
    let (shown, _) = page_in(host, parent, Visibility::Shown)?;
    let lit = shows_before(
        connection,
        (parent, WHOLE),
        |lit| lit > 0,
        Instant::now() + KEY_WAIT,
    )?;
    assert!(lit, "the first page never painted");
    let (spare, messages) = page_in(host, parent, Visibility::Hidden)?;
    spare.eval(DARK_WHILE_HIDDEN)?;
    wait_for(&messages, "dark", PAGE_WAIT)?;
    spare.show()?;
    let dark = shows_before(
        connection,
        (parent, WHOLE),
        |lit| lit == 0,
        Instant::now() + KEY_WAIT,
    )?;
    assert!(dark, "the spare page never painted over the first");
    spare.destroy()?;
    shown.destroy()?;
    Ok(())
}
