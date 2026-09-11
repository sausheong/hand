# Reading long output

Press **Esc** to cancel the current turn, including while it is waiting for tool approval, checking results or summarising context. Esc also cancels a running turn when an output viewer is open. Hand stays open. While idle, Esc keeps its usual menu/viewer behaviour. Ctrl+C remains available.

Scroll the conversation with a mouse wheel or trackpad. Page Up/Down and Ctrl+U/D also scroll it. Scrolling up keeps your reading position when new output arrives; scrolling back to the bottom resumes following new output.

Mouse scrolling is on by default. If your terminal captures mouse drags instead of selecting text, use `/mouse off` to select and copy normally. Keyboard scrolling still works. Use `/mouse on` to restore wheel scrolling. This choice lasts for the current Hand process.

The full tool-output viewer also supports wheel scrolling. Clicks never submit messages or approve commands.

`/skills` uses bold blue names, normal-colour descriptions, muted source paths, and a blank line between entries. Long descriptions and source paths remain available in full.

With iTerm2, `/mouse on` also supports native selection: hold **Option** while
dragging. To select normally without a modifier, open **Settings → Profiles →
Terminal**, keep **Enable mouse reporting** and **Report mouse wheel events** on,
and turn **Report mouse clicks & drags** off. Hand handles wheel events and does
not use clicks or drags for actions. These are terminal settings; Hand cannot
force native selection while the terminal is sending it mouse events.
See [iTerm2's mouse reporting documentation](https://iterm2.com/documentation-preferences-profiles-terminal.html).

Bash tool headers show a command preview once its arguments arrive. The preview
contains the first five command lines, clips long lines to the terminal width,
and marks omitted lines. Saved-session replay uses the same preview. The command
itself is executed unchanged; the five-line limit affects only its display.

The conversation and footer are separated by a horizontal rule. The status or
approval area has a blank line before the input border, and its wrapped height
is reserved so long status text does not displace the editor.

During interactive use, raw warning/error logs are written to private
`~/.hand/logs/interactive-*.log` files instead of the terminal. User-facing tool
failures and application errors remain in the conversation. Each invocation has
its own log file; one-shot mode retains its existing terminal diagnostics.
