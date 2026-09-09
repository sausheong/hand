# Image input in staged terminal and one-shot execution

Image paths in a normal terminal prompt can be quoted or use escaped spaces:

```text
Review "screenshots/home page.png"
Compare @"screenshots/home page.png" with screenshots/settings\ page.png
```

PNG, JPEG, GIF and WebP extensions are recognised. Repeated references to the
same normalised workspace path attach once. The displayed prompt uses image
placeholders; the original prompt text and image bytes are supplied to the run.

A missing, unreadable, external, non-regular or oversized image prevents the
submission and leaves the input draft intact. Limits are 5 MiB per image,
20 MiB of image bytes per prompt and 16 unique paths. Workspace-rooted reads
reject symlink traversal outside the workspace. A quoted image path with an
unfinished quote is an error.

The same parser is used by one-shot execution, for example:

```sh
hand -p 'Review "screenshots/home page.png"'
```

The selected profile must accept images. Missing/invalid attachments prevent
provider invocation; cancellation keeps the cancelled outcome.

External attachment approval, content-based image validation and queued image
resolution remain under implementation. `/steer` and `/followup` still carry
text. Generic `@file` references and path completion remain pending.
