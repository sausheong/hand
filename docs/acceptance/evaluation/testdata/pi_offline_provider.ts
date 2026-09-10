// Deterministic transport fixture. No network, credentials or real model calls.
import { createAssistantMessageEventStream } from "@earendil-works/pi-ai";

export default function (pi: any) {
  let calls = 0;
  pi.registerProvider("hand-offline-fixture", {
    api: "hand-offline-fixture",
    apiKey: "offline-fixture-only",
    baseUrl: "http://127.0.0.1:1",
    models: [{ id: "offline-fixture", name: "Offline fixture", reasoning: false,
      input: ["text"], contextWindow: 4096, maxTokens: 128,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } }],
    streamSimple(model: any, context: any, options: any) {
      const stream = createAssistantMessageEventStream();
      const prompt = JSON.stringify(context.messages);
      const attempt = ++calls;
      const message: any = { role: "assistant", content: [], api: model.api,
        provider: model.provider, model: model.id, timestamp: Date.now(),
        usage: { input: 4, output: 2, cacheRead: 0, cacheWrite: 0, totalTokens: 6,
          cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
        stopReason: "pending" };
      queueMicrotask(() => {
        stream.push({ type: "start", partial: message });
        if (prompt.includes("WAIT_FOR_ABORT")) {
          const abort = () => {
            message.stopReason = "aborted";
            message.errorMessage = "Offline fixture cancelled";
            stream.push({ type: "error", reason: "aborted", error: message });
            stream.end();
          };
          if (options.signal?.aborted) abort();
          else options.signal?.addEventListener("abort", abort, { once: true });
          return;
        }
        if (prompt.includes("RETRY_ONCE") && attempt === 1) {
          message.stopReason = "error";
          message.errorMessage = "503 Service Unavailable: offline retry fixture";
          stream.push({ type: "error", reason: "error", error: message });
        } else {
          message.content = [{ type: "text", text: "offline fixture complete" }];
          message.stopReason = "stop";
          stream.push({ type: "done", reason: "stop", message });
        }
        stream.end();
      });
      return stream;
    },
  });
}
