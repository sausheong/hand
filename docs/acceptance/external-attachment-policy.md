# Explicit external attachments

The staged integration adds repeatable `--allow-attachment PATH` grants. Each grant snapshots one exact file for the current invocation. Use an explicit `@` reference to include it:

```sh
hand --allow-attachment '/tmp/reference notes.txt' -p 'Use @"/tmp/reference notes.txt" to explain the change'
hand --allow-attachment '/tmp/screenshot.png'
```

In the interactive example, enter `inspect @"/tmp/screenshot.png"`. Grants apply to ordinary prompts, steering and follow-ups. A grant alone does not send the file. Bare external image paths remain denied. The snapshot remains unchanged if the original file is later edited or removed; restart with a new grant to refresh it.

Grants do not change tool permissions, search roots or path completion. There are no directory or wildcard grants, and workspace configuration cannot create grants. Missing files, directories, special files and oversized files fail admission. Limits are 16 granted files, 5 MiB per file and 20 MiB combined. Existing text limits (256 KiB per reference and 1 MiB per prompt) still apply when attaching text.

Grants are held in memory, copied when consumed and discarded at process exit. Attached content becomes part of the conversation and can be persisted in the session. Only select files intended for the configured model.

This implementation is staged against an unpublished Harness candidate. Native qualification and complete M3.2 acceptance are pending.
