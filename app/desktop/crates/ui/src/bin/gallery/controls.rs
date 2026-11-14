//! Buttons, inputs, toggles and tabs.

use gpui::{Context, IntoElement, ParentElement, Styled, div, px};
use ui::{
    Button, ButtonSize, ButtonVariant, Checkbox, IconName, Select, Surface, Switch, TabItem, Tabs, TabsStyle,
};

use crate::page::Page;

const VARIANTS: [(ButtonVariant, &str); 5] = [
    (ButtonVariant::Primary, "Primary"),
    (ButtonVariant::Destructive, "Delete"),
    (ButtonVariant::Outline, "Outline"),
    (ButtonVariant::Secondary, "Secondary"),
    (ButtonVariant::Ghost, "Ghost"),
];
const SIZES: [(ButtonSize, &str); 5] = [
    (ButtonSize::Xs, "xs"),
    (ButtonSize::Sm, "sm"),
    (ButtonSize::Md, "md"),
    (ButtonSize::Lg, "lg"),
    (ButtonSize::Xl, "xl"),
];

impl Page {
    pub(crate) fn buttons(&self, _cx: &Context<Self>) -> impl IntoElement {
        let variants = VARIANTS.iter().fold(div().flex().gap(px(8.0)), |row, (variant, label)| {
            row.child(Button::new(*label, *label).variant(*variant))
        });
        let disabled = VARIANTS.iter().fold(div().flex().gap(px(8.0)), |row, (variant, label)| {
            row.child(Button::new(("off", *label), *label).variant(*variant).disabled(true))
        });
        let sizes = SIZES.iter().fold(div().flex().items_center().gap(px(8.0)), |row, (size, label)| {
            row.child(Button::new(("size", *label), *label).size(*size).variant(ButtonVariant::Outline))
        });
        let icons = SIZES.iter().fold(div().flex().items_center().gap(px(8.0)), |row, (size, label)| {
            row.child(Button::icon_only(("icon", *label), IconName::Gear).size(*size))
        });
        let with_icon = div().flex().gap(px(8.0)).child(Button::new("add", "Add space").icon(IconName::Plus));
        div().flex().flex_col().gap(px(8.0)).child(variants).child(disabled).child(sizes).child(icons).child(with_icon)
    }

    pub(crate) fn inputs(&self, cx: &Context<Self>) -> impl IntoElement {
        let page = cx.entity();
        let options = ["System", "Light", "Dark"].map(Into::into).to_vec();
        div()
            .flex()
            .items_start()
            .gap(px(16.0))
            .child(div().w(px(240.0)).child(self.input.clone()))
            .child(Select::new("select", options, self.choice).width(px(160.0)).on_change(move |index, _, cx| {
                page.update(cx, |page, cx| {
                    page.choice = Some(index);
                    cx.notify();
                });
            }))
    }

    pub(crate) fn toggles(&self, cx: &Context<Self>) -> impl IntoElement {
        let (on_switch, on_check, on_tab, on_segment) = (cx.entity(), cx.entity(), cx.entity(), cx.entity());
        let tabs = vec![TabItem::icon(IconName::ChatsCircle), TabItem::icon(IconName::FolderOpen), TabItem::icon(IconName::GitBranch)];
        let segments = vec![TabItem::text("Chat"), TabItem::text("Terminal")];
        div()
            .flex()
            .flex_col()
            .gap(px(12.0))
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(16.0))
                    .child(Switch::new("switch", self.switch_on).on_change(move |value, _, cx| {
                        on_switch.update(cx, |page, cx| {
                            page.switch_on = value;
                            cx.notify();
                        });
                    }))
                    .child(Switch::new("switch-off", false))
                    .child(Switch::new("switch-disabled", true).disabled(true))
                    .child(Checkbox::new("check", self.checked).on_change(move |value, _, cx| {
                        on_check.update(cx, |page, cx| {
                            page.checked = value;
                            cx.notify();
                        });
                    }))
                    .child(Checkbox::new("check-off", false))
                    .child(Checkbox::new("check-disabled", true).disabled(true)),
            )
            .child(div().w(px(294.0)).child(Tabs::new("tabs", tabs, self.tab).on(Surface::Window).on_select(move |index, _, cx| {
                on_tab.update(cx, |page, cx| {
                    page.tab = index;
                    cx.notify();
                });
            })))
            .child(div().w(px(200.0)).child(Tabs::new("segments", segments, self.segment).style(TabsStyle::Segmented).on_select(
                move |index, _, cx| {
                    on_segment.update(cx, |page, cx| {
                        page.segment = index;
                        cx.notify();
                    });
                },
            )))
    }
}
