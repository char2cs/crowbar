# Sidebar states at 1200x800. Coordinates are client pixels of the default seeded layout.
# Menus are native OS windows on Linux/macOS, so they are captured with shot_root.
key ctrl+1; settle
shot sb-01-chats-default "Chats mode, selected chat row"
move 90 192; shot sb-02-row-hover "hover on the 'main' workspace row"
move 90 230; shot sb-03-chat-row-hover "hover on selected chat row (actions appear)"
move 56 313; shot sb-04-more-hover "hover on More..."
move 150 63; shot sb-05-header-hover "hover on project header (buttons appear)"
move 26 21; shot sb-06-gear-hover "settings gear hover"
move 272 21; shot sb-07-toggle-hover "sidebar toggle hover"
click 56 313; shot_root sb-10-more-menu "More... menu open"; key Escape; settle
rclick 55 154; shot_root sb-11-space-menu "right-click space header"; key Escape; settle
rclick 64 192; shot_root sb-12-workspace-menu "right-click workspace row"; key Escape; settle
rclick 90 230; shot_root sb-13-chat-menu "right-click chat row"; key Escape; settle
click 147 110; shot sb-20-files-mode "Files mode"
rclick 60 192; shot_root sb-21-files-menu "right-click file row"; key Escape; settle
click 239 110; shot sb-22-changes-mode "Changes mode"
click 55 110; shot sb-23-chats-again "Chats again"
click 72 275; shot sb-30-new-thread "after New thread click"
