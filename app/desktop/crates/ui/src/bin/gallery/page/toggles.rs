//! Switch, checkbox and the two tab styles.

use gpui::{Entity, IntoElement, ParentElement, Pixels, Styled, div, px};
use ui::{Checkbox, IconName, Surface, Switch, TabItem, Tabs, TabsStyle, spacing};

use crate::page::Page;
use crate::page::section::row;

const PILL_WIDTH: Pixels = px(294.0);
const SEGMENT_WIDTH: Pixels = px(200.0);

fn switches(
    page: &Page,
    entity: &Entity<Page>,
) -> impl IntoElement {
    let owner = entity.clone();
    let switch = Switch::new("switch", page.switch_on).on_change(move |value, _, cx| {
        let on = bool::from(value);
        owner.update(cx, |page, cx| page.edit(cx, |page| page.switch_on = on));
    });
    row()
        .gap(spacing::STEP_4)
        .child(switch)
        .child(Switch::new("switch-off", false))
        .child(Switch::new("switch-disabled", true).disabled())
}

fn checkboxes(
    page: &Page,
    entity: &Entity<Page>,
) -> impl IntoElement {
    let owner = entity.clone();
    let checkbox = Checkbox::new("check", page.checked).on_change(move |value, _, cx| {
        let on = bool::from(value);
        owner.update(cx, |page, cx| page.edit(cx, |page| page.checked = on));
    });
    row()
        .gap(spacing::STEP_4)
        .child(checkbox)
        .child(Checkbox::new("check-off", false))
        .child(Checkbox::new("check-disabled", true).disabled())
}

fn pill_tabs(
    page: &Page,
    entity: &Entity<Page>,
) -> impl IntoElement {
    let owner = entity.clone();
    let items = vec![
        TabItem::icon(IconName::ChatsCircle),
        TabItem::icon(IconName::FolderOpen),
        TabItem::icon(IconName::GitBranch),
    ];
    let tabs = Tabs::new("tabs", items, page.tab)
        .on(Surface::Window)
        .on_select(move |index, _, cx| {
            owner.update(cx, |page, cx| page.edit(cx, |page| page.tab = index));
        });
    div().w(PILL_WIDTH).child(tabs)
}

fn segmented_tabs(
    page: &Page,
    entity: &Entity<Page>,
) -> impl IntoElement {
    let owner = entity.clone();
    let items = vec![TabItem::text("Chat"), TabItem::text("Terminal")];
    let tabs = Tabs::new("segments", items, page.segment)
        .style(TabsStyle::Segmented)
        .on_select(move |index, _, cx| {
            owner.update(cx, |page, cx| page.edit(cx, |page| page.segment = index));
        });
    div().w(SEGMENT_WIDTH).child(tabs)
}

pub(crate) fn toggles(
    page: &Page,
    entity: &Entity<Page>,
) -> impl IntoElement {
    div()
        .flex()
        .flex_col()
        .gap(spacing::STEP_3)
        .child(switches(page, entity))
        .child(checkboxes(page, entity))
        .child(pill_tabs(page, entity))
        .child(segmented_tabs(page, entity))
}
