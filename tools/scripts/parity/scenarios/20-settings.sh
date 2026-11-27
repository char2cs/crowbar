# Settings modal: open from the sidebar gear, every page, then search, select popup, scroll.
click 26 21; settle 0.8; shot set-01-appearance "modal open, Appearance page"
move 140 170; shot set-02-nav-hover "nav rail hover (Editor)"
click 124 170; shot set-10-editor "Editor page"
click 117 204; shot set-11-files "Files page"
click 112 238; shot set-12-git "Git page"
click 132 272; shot set-13-terminal "Terminal page"
click 129 306; shot set-14-browser "Browser page"
click 125 340; shot set-15-agents "Agents page"
click 149 374; shot set-16-active-sessions "Active sessions page"
click 143 408; shot set-17-keybindings "Keybindings page"
scroll 700 400 5 8; shot set-18-keybindings-scrolled "Keybindings scrolled"
click 120 442; shot set-19-hosts "Hosts page"
click 140 136; shot set-20-appearance-again "back to Appearance"
scroll 700 400 5 6; shot set-21-appearance-scrolled "Appearance scrolled down"
scroll 700 400 4 12
click 1024 153; shot_root set-22-select-open "Color Theme select open"; key Escape; settle
click 258 86; type_ "font"; shot set-23-search "search 'font'"
key ctrl+a; key BackSpace; key Escape; settle 0.6; shot set-24-search-cleared-esc "search cleared / Escape"
click 1110 86; settle 0.6; shot set-30-closed "closed via X"
record setm-open 1 30 -- click 26 21
record setm-close 1 30 -- key Escape
