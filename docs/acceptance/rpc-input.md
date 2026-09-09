# RPC initial prompt resolution

RPC prompt submission now calls Service.StartInput, which uses the application's configured resolver before admitting a run. Production controller construction supplies the same workspace/attachment-policy resolver used by follow-ups. A workspace `@file` therefore contributes its resolved content instead of reaching the model as an unresolved reference.

Resolution does not hold the application mutex. Admission rechecks the selected session and current model input capabilities before creating the owned stream. A session change during resolution rejects the input rather than applying it to another conversation. Image capabilities are checked after resolution, and the existing stream copies admitted image data.

Permanent tests verify actual workspace text-file inclusion through RPC, rejection when session selection changes during resolution, and rejection of resolved images for text-only profiles. RPC/application race-suite evidence and vet output are in `rpc-input-integration.json`. The first application run was blocked by the sandbox's local-port restriction in an existing metadata HTTP fixture; it is retained separately from the successful run with fixture access enabled.

These checks do not replace a full provider image journey, native attachment qualification or final M4 acceptance. Automatic RPC follow-up continuation and complete client journeys remain pending.
