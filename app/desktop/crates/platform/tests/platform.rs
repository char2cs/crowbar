#[cfg(target_os = "macos")]
fn main() -> Result<(), String> {
    if std::env::args().any(|arg| arg == "--ignored" || arg == "--include-ignored") {
        return appkit::run();
    }
    Ok(())
}

#[cfg(not(target_os = "macos"))]
fn main() {}

#[cfg(target_os = "macos")]
mod appkit {
    use objc2::MainThreadMarker;
    use objc2::rc::Retained;
    use objc2_app_kit::{NSApplication, NSView, NSViewController, NSWindow};
    use objc2_foundation::{NSPoint, NSRect, NSSize};
    use platform::{AppkitParent, ClipShape, ContextMenu, Frost, MenuRow, PageClip};
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    pub(crate) fn run() -> Result<(), String> {
        let main_thread =
            MainThreadMarker::new().ok_or("the AppKit cases must run on the main thread")?;
        finds_a_nested_view_of_an_open_window_and_presents_it(main_thread)?;
        a_view_no_open_window_shows_is_not_found(main_thread)?;
        frosting_puts_one_blur_view_under_the_window_s_content(main_thread)?;
        a_page_clip_takes_the_pointer_only_where_its_page_shows(main_thread)?;
        a_page_clip_wears_a_mask_that_counts_from_its_top(main_thread)?;
        a_context_menu_draws_its_rows_and_names_the_one_picked(main_thread)
    }

    fn row(label: &str) -> MenuRow {
        MenuRow::Item {
            label: label.into(),
            enabled: true,
            checked: false,
        }
    }

    fn a_context_menu_draws_its_rows_and_names_the_one_picked(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let nested = MenuRow::Submenu {
            label: "Move to".into(),
            enabled: true,
            rows: vec![row("main")],
        };
        let rows = [
            MenuRow::Item {
                label: "Cut & Paste".into(),
                enabled: false,
                checked: false,
            },
            MenuRow::Separator,
            MenuRow::Item {
                label: "Wrap".into(),
                enabled: true,
                checked: true,
            },
            nested,
        ];
        let menu = ContextMenu::new(&rows, main_thread);
        let want = [
            ("Cut & Paste".to_owned(), false, 0),
            (String::new(), false, 0),
            ("Wrap".to_owned(), true, 1),
            ("Move to".to_owned(), true, 0),
        ];
        if drawn(&menu) != want || menu.picked().is_some() {
            return Err(format!("the menu was drawn as {:?}", drawn(&menu)));
        }
        picks_in_the_submenu(&menu)
    }

    fn drawn(menu: &ContextMenu) -> Vec<(String, bool, isize)> {
        menu.menu()
            .itemArray()
            .to_vec()
            .iter()
            .map(|item| (item.title().to_string(), item.isEnabled(), item.state()))
            .collect()
    }

    fn picks_in_the_submenu(menu: &ContextMenu) -> Result<(), String> {
        let under = menu
            .menu()
            .itemAtIndex(3)
            .and_then(|item| item.submenu())
            .ok_or("no submenu")?;
        under.performActionForItemAtIndex(0);
        match menu.picked() {
            Some(path) if path == [3, 0] => Ok(()),
            other => Err(format!("picked {other:?}")),
        }
    }

    const HOLE: [f64; 4] = [60.0, 0.0, 69.0, 20.0];

    type Clipped = (Retained<NSWindow>, Retained<NSView>, PageClip);

    fn clipped(main_thread: MainThreadMarker) -> Result<Clipped, String> {
        let frame = NSRect::new(NSPoint::new(0.0, 0.0), NSSize::new(300.0, 200.0));
        let content = NSView::initWithFrame(main_thread.alloc(), frame);
        let controller = NSViewController::new(main_thread);
        controller.setView(&content);
        let window = NSWindow::windowWithContentViewController(&controller);
        let parent = AppkitParent::find(Retained::as_ptr(&content).addr(), main_thread)
            .ok_or("the content view was not found")?;
        let clip = PageClip::within(&parent).map_err(|error| error.to_string())?;
        clip.set_frame([0.0, 0.0, 200.0, 100.0]);
        clip.set_shape(ClipShape {
            corners: [9.0, 0.0, 0.0, 9.0],
            hole: Some(HOLE),
        });
        Ok((window, content, clip))
    }

    fn view_of_clip(content: &NSView) -> Result<Retained<NSView>, String> {
        content
            .subviews()
            .to_vec()
            .pop()
            .ok_or("no clip view".into())
    }

    fn is(
        found: Option<Retained<NSView>>,
        want: &NSView,
    ) -> bool {
        let want = std::ptr::from_ref(want);
        found.is_some_and(|found| std::ptr::eq(Retained::as_ptr(&found), want))
    }

    fn a_page_clip_takes_the_pointer_only_where_its_page_shows(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let (_window, content, _clip) = clipped(main_thread)?;
        let view = view_of_clip(&content)?;
        let page = NSView::initWithFrame(
            main_thread.alloc(),
            NSRect::new(NSPoint::new(0.0, 0.0), NSSize::new(200.0, 50.0)),
        );
        view.addSubview(&page);
        let hit = |x: f64, y: f64| content.hitTest(NSPoint::new(x, 200.0 - y));
        if !is(hit(20.0, 30.0), &page) {
            return Err("the page does not take the pointer".into());
        }
        if !is(hit(90.0, 5.0), &content) {
            return Err("the hole does not pass the pointer through".into());
        }
        if !is(hit(20.0, 80.0), &content) {
            return Err("the bare clip takes the pointer".into());
        }
        Ok(())
    }

    fn a_page_clip_wears_a_mask_that_counts_from_its_top(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let (_window, content, _clip) = clipped(main_thread)?;
        let view = view_of_clip(&content)?;
        let masked = view
            .layer()
            .is_some_and(|layer| layer.mask().is_some() && layer.contentsAreFlipped());
        match (masked, view.isFlipped()) {
            (true, true) => Ok(()),
            _ => Err("the clip is not masked from its top".into()),
        }
    }

    fn frosting_puts_one_blur_view_under_the_window_s_content(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let content = NSView::new(main_thread);
        let drawn = NSView::new(main_thread);
        content.addSubview(&drawn);
        let controller = NSViewController::new(main_thread);
        controller.setView(&content);
        let _window = NSWindow::windowWithContentViewController(&controller);
        let address = Retained::as_ptr(&drawn).addr();
        for frost in [Frost::Dark, Frost::Light] {
            platform::frost_window(address, frost).map_err(|error| error.to_string())?;
        }
        let under = content.subviews().to_vec();
        match under.as_slice() {
            [_, above] if Retained::as_ptr(above).addr() == address => Ok(()),
            other => Err(format!(
                "expected one blur view below the drawn view, got {other:?}"
            )),
        }
    }

    fn finds_a_nested_view_of_an_open_window_and_presents_it(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let _app = NSApplication::sharedApplication(main_thread);
        let content = NSView::new(main_thread);
        let nested = NSView::new(main_thread);
        content.addSubview(&nested);
        let controller = NSViewController::new(main_thread);
        controller.setView(&content);
        let _window = NSWindow::windowWithContentViewController(&controller);
        let address = Retained::as_ptr(&nested).addr();

        let parent = AppkitParent::find(address, main_thread)
            .ok_or("the nested view of an open window was not found")?;
        let handle = parent
            .window_handle()
            .map_err(|error| format!("the parent has no window handle: {error}"))?
            .as_raw();
        match handle {
            RawWindowHandle::AppKit(appkit) if appkit.ns_view.as_ptr().addr() == address => Ok(()),
            other => Err(format!(
                "expected the nested view as an AppKit handle, got {other:?}"
            )),
        }
    }

    fn a_view_no_open_window_shows_is_not_found(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let loose = NSView::new(main_thread);
        let address = Retained::as_ptr(&loose).addr();
        match AppkitParent::find(address, main_thread) {
            None => Ok(()),
            Some(parent) => Err(format!("a view outside every window was found: {parent:?}")),
        }
    }
}
