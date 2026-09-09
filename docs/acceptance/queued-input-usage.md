# Queued follow-ups in the staged terminal

While a goal is running, enter `/followup <message>` to add work that should run
once the current goal and its completion checks finish successfully. Multiline
message text is retained. When idle, the same command starts its follow-up.

Use `/queue` to see pending entries and their IDs. Use `/queue edit <ID> <text>`
to replace an entry's text, or `/queue remove <ID>` to cancel that pending entry.
Edits retain the ID and ordering. `/queue run` starts the oldest pending follow-up
when the application is idle.

Cancelling a run leaves pending follow-ups queued. A failed run also pauses
automatic dispatch. Inspect/edit the pending entries and use `/queue run` when
ready. Already admitted runs are not automatically requeued, since they may have
performed work before cancellation.

During tool approval, typing a slash command enters command input without
answering the approval. Only queue commands may be submitted in that input mode;
Escape clears the command and returns to the approval keys. Queue submission
and removal leave the tool approval pending.

Current limits are 64 queued entries and 64 KiB of UTF-8 text per entry. Queues
are process-local and are not restored after application exit. These controls
now resolve image paths and @file references for follow-ups when they are
admitted. File changes made while a message waits are therefore reflected in
its eventual snapshot. Missing/invalid attachments or an image-incompatible
profile leave the entry queued. Steering attachments now resolve when delivery is claimed; durable queue
restart remains under implementation.

This functionality is in the staged integration patch, pending integration of
the unpublished Harness dependency. See `input-queue-tui-integration.md` for
verification and limitations.

## Steering

Use `/steer <correction>` during a running goal to deliver text at a safe tool
boundary. An already running tool or pending approval is not interrupted. Once
that tool settles, remaining proposed tools are skipped and the model receives
the correction. `/queue` shows pending and claimed corrections separately.

Claimed corrections cannot be edited or removed until delivery is acknowledged
or verified after the run joins. When a successful flush confirms no correction
was written, the entry becomes pending and editable again. Confirmed written
corrections leave the queue. A persistence failure leaves claims visibly
unresolved. Pending corrections remain editable. Steering queued while idle is delivered during the next application
run. Hand enables serial tool execution while using this boundary so remaining
tools cannot race a queued correction.

Attachment reads occur outside the application owner lock. The selected entry,
its text, session and queue order are checked again before starting the run. If
you edit or remove the entry during resolution, admission fails rather than
sending the old snapshot. Edit the pending entry or use `/queue run` to retry.

Steering captures images and @file contents once when it claims a correction.
That snapshot remains unchanged through acknowledgement retries, even if the
source files change. Missing attachments or incompatible image profiles fail
before claiming the entry, leaving it editable. A confirmed unwritten claim
releases its snapshot so a subsequent attempt can resolve the current files.
