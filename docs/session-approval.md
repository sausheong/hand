# Approve all tools for a session

Between turns, use these commands inside Hand:

- `/permissions skip` automatically approves all tools for the rest of this process.
- `/permissions ask` restores normal approval behaviour, including existing saved grants.
- `/permissions` shows the current mode and saved grants.

The commands and startup flag control the same setting. `/permissions ask` also turns off a bypass enabled at launch. Finish or cancel an active turn before changing modes. Neither command writes or deletes saved grants.

Start Hand with:

```sh
hand --dangerously-skip-permissions
```

This automatically approves all tool requests, including Bash, file edits, MCP tools and extension tools, for this Hand process. It works in interactive, one-shot and RPC modes. The terminal shows `approvals off` while the option is active.

No permanent grants are written. The option remains active across turns and `/new` or `/resume` within the same process. Quit and launch Hand without the flag to return to normal approvals; resuming a saved session does not restore the bypass.

The flag changes tool approval only. Execution isolation, tool argument validation, lifecycle/extension policies and spending limits still apply. It does not approve configuration reviews or grant attachment access outside the existing attachment policy.

For example:

```sh
hand --dangerously-skip-permissions -p "Run the tests and fix the failures"
```

The existing `--yes` option continues to work for one-shot requests with `-p`.
