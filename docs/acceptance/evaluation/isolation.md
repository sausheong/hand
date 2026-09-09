# Isolated evaluation-agent boundary

The retained-scope probe runs the actual Hand executable in a non-root container
with a read-only root filesystem, no capabilities, no new privileges, bounded
resources and Docker `network=none`. Its tools run inside that outer container.
Hand's internal “host” label is relative to this container, not the developer's
machine. This is distinct from ordinary Hand container-tool mode, where provider
requests remain on the host and that boundary is explicitly reported.

A local TCP-to-Unix bridge inside the agent connects only to the evaluation
gateway's Unix socket. The socket lives in an owned Docker volume mounted
read-only by the agent. A separate gateway container owns the writable socket,
private configuration and budget database. The agent receives only its run token.
The existing GatewayController and GatewayHTTPServer enforce authentication,
fixed model/route/authority, bounded framing and request admission. The probe's
upstream is a socket-pair fixture; neither container makes a provider call.

The executable probe asserts:

- A real Hand read-file turn completes, and its tool result reaches the second
  admitted provider request. Exactly two requests consume admission slots.
- Incorrect tokens, model substitutions, foreign Host headers and absolute
  upstream request targets are rejected without additional upstream requests.
- The agent cannot read the private gateway configuration, budget database,
  Docker socket or host sentinel. The synthetic provider secret is absent from
  accessible process environments and agent files.
- A separate networked container first connects to a live sentinel. The agent
  cannot reach that sentinel or an external IP. It has no IPv4 routes and no
  active external interfaces or non-loopback IPv6 addresses. Dormant Linux
  tunnel devices do not establish connectivity.
- Gateway and proxy threads join; all three owned containers and the owned
  socket volume are removed. Source and binary hashes remain unchanged.

Run `scripts/check_evaluation_isolation.py --help` for required inputs. Supply a
Linux Hand binary linked to published Harness v0.4.0, a loaded immutable Python
image ID of the matching architecture, the explicit Docker executable/socket,
the repository root and a fresh output directory. The helper neither pulls images
nor starts a daemon, and never prunes shared Docker state. It generates synthetic
credentials in a private directory; do not supply real provider credentials.

This establishes the controlled boundary, not a complete comparison executor,
live-provider billing correctness, model quality or superiority to Pi. The
omitted comparison work is not restored by this probe.
