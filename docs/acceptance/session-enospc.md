# Reproduce the bounded full-disk export check

The test fills only an explicitly supplied tmpfs of at most 8 MiB. The recorded run used a 2 MiB mount, networking disabled and a read-only container root. It checks the real Hand CLI diagnostic, unchanged source/catalogue, absence of partial output and successful retry after freeing space. [Recorded evidence](session-enospc.json) includes binary and image hashes.

From the Hand repository, with its intended Harness dependency resolved, build Linux arm64 artifacts:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /private/tmp/hand-enospc ./cmd/hand
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o /private/tmp/hand-enospc.test ./cmd/hand
```

Run using the reviewed Debian image already available in the local Docker store:

```sh
docker run --rm --pull=never --platform=linux/arm64 \
  --network none --read-only \
  --tmpfs /case:rw,size=2m,mode=0700 \
  --tmpfs /tmp:rw,size=32m,mode=1777 \
  --mount type=bind,source=/private/tmp/hand-enospc.test,target=/test,readonly \
  --mount type=bind,source=/private/tmp/hand-enospc,target=/hand,readonly \
  -e HAND_TEST_ENOSPC_ROOT=/case \
  -e HAND_TEST_ENOSPC_BINARY=/hand \
  --entrypoint /test \
  sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818 \
  -test.run='^TestBinary(Export|Conversation)ReportsRealENOSPC$' -test.v
```

Require exit zero and the named passing test, retaining the entire log. An unavailable image, missing input or skipped test is not a pass. The image ID is platform-specific; another architecture requires an independently recorded image and matching binary builds. These CGO-disabled binaries are not race-instrumented.

These checks cover export publication and active conversation persistence under genuine ENOSPC. The conversation case fills the tmpfs only after its local provider fixture receives a request, verifies the CLI failure, then frees space and reopens the retained questions with a usable writer. No external provider is called. [Conversation evidence](conversation-enospc.json) records the combined run. Interactive UI rendering and native macOS behaviour remain unqualified. Final qualification must repeat it against the frozen Hand candidate and released Harness dependency; record fresh hashes instead of reusing the development artifact identities.
