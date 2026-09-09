# External editor in the staged terminal

Press **Ctrl+G** while the application is idle to edit the current input. Hand
uses `VISUAL`, then `EDITOR`, and falls back to `vi`. The editor must remain open
until editing is finished; for example, set `VISUAL='code --wait'` for VS Code.
Quoted executable paths and arguments are supported. Shell expansion,
substitution, pipelines and redirection are not evaluated.

The terminal is suspended through Bubble Tea's ExecProcess and restored after
the editor exits. Edited text returns to the input box; it is not submitted.
A private temporary directory contains the input file with mode 0600. The
normal completion/failure callback removes the temporary directory.

Output must be a regular UTF-8 text file no larger than 64 KiB, without NUL
bytes. If the editor fails, output is invalid, or the textarea would alter or
truncate the text, Hand reports the issue and retains the original draft.
The editor cannot start while a goal, approval, compaction or configuration
operation is active.

Process-kill cleanup and interactive native-terminal qualification remain
pending. The automated round-trip uses a real editor subprocess and tests the
result application separately; it does not prove every third-party editor's
terminal behaviour. See `external-editor-integration.md` for evidence.
