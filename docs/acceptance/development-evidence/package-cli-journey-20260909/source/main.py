"""Persistent task-note peer. Stdout is reserved for bounded protocol frames."""
import json
import sys

MAX_FRAME = 256 * 1024
sequence = 0


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def receive():
    line = sys.stdin.buffer.readline(MAX_FRAME + 2)
    if not line:
        raise EOFError
    if not line.endswith(b"\n") or len(line) - 1 > MAX_FRAME:
        raise ValueError("invalid frame length")
    frame = json.loads(line.decode("utf-8"), object_pairs_hook=unique_object)
    if frame.get("version") != 1 or not isinstance(frame.get("id"), str):
        raise ValueError("invalid protocol envelope")
    return frame


def send(frame):
    data = json.dumps(frame, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    if len(data) > MAX_FRAME:
        raise ValueError("response too large")
    sys.stdout.buffer.write(data + b"\n")
    sys.stdout.buffer.flush()


def callback(method, params):
    global sequence
    sequence += 1
    request_id = f"note-{sequence}"
    send(dict(version=1, kind="request", id=request_id, method=method, params=params))
    reply = receive()
    if reply.get("kind") != "response" or reply["id"] != request_id or "error" in reply:
        raise ValueError("host callback refused or invalid")
    return reply["result"]


def handle(frame):
    method, params = frame["method"], frame["params"]
    if method == "initialize":
        return dict(version=1, name="task-note", capabilities=["commands", "questions", "state", "context.transform"], commands=[dict(name="note", description="Ask for and remember a task note")])
    if method == "command.execute":
        if params.get("name") != "note":
            raise ValueError("unknown command")
        answer = callback("user.question", dict(id="note", title="What task note should be remembered?", allow_free_text=True))
        if answer.get("id") != "note":
            raise ValueError("wrong question answer")
        if answer.get("cancelled"):
            return dict(blocks=[dict(kind="text", text="Note cancelled.")])
        if not isinstance(answer.get("text"), str) or answer.get("choice"):
            raise ValueError("expected text answer")
        state = callback("state.get", {})
        callback("state.set", dict(revision=state["revision"], data=dict(note=answer["text"])))
        return dict(blocks=[dict(kind="text", text="Task note saved.")])
    if method == "context.transform":
        return dict(replacements=[dict(id=item["id"], text=item["text"].strip()) for item in params["items"]])
    raise ValueError("unsupported method")


def main():
    while True:
        try:
            frame = receive()
        except EOFError:
            return
        if frame.get("kind") != "request":
            raise ValueError("expected request")
        reply = dict(version=1, kind="response", id=frame["id"])
        try:
            reply["result"] = handle(frame)
        except (ValueError, KeyError, TypeError) as error:
            print(str(error), file=sys.stderr)
            reply["error"] = dict(code="request_failed", message="Task note request failed; see extension diagnostics.")
        send(reply)


if __name__ == "__main__":
    main()
