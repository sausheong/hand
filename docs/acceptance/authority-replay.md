# Authority revocation replay

Authority loading now reconstructs grant/revocation history before consulting current filesystem resources. Previously, a revoked directory replaced with a symlink could prevent the entire authority store from reopening, despite that grant no longer being active.

Historical records still undergo schema, context, provenance, lifetime and canonical path-shape validation. After revocations are applied, remaining active filesystem grants must still resolve to their saved canonical resources. Changed active grants require reapproval; revoked grants cannot regain authority.

A permanent regression grants and revokes a directory, preserves an unrelated grant, replaces the revoked directory with an external symlink, then reopens. It verifies the unrelated grant remains usable and the revoked scope remains denied. The permission race suite and vet pass. Evidence: `authority-replay-integration.json`. Production approval integration and final acceptance remain pending.
