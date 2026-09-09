# Harness strict session records — local evidence

Commit `ce578590c0e38b3e204bc10b17fb995759449441` follows writer-lease candidate `dfae3db0f2f016089e4be29f14d97bee40f24e4b` in `/private/tmp/hand-harness-persistence-20260908`. It is local/unpublished and Hand still uses released v0.3.9.

Store.Load no longer skips malformed records and returns a partial session. The bounded decoder returns RecordError with a line number, byte offset and recovery classification. Each record is limited to 10 MiB; invalid UTF-8, trailing JSON values and non-object records are rejected. A recovery candidate must be an unfinished JSON object at physical EOF. An invalid or newline-terminated record, including an incomplete object before later records, is not eligible for tail removal. Loading never modifies the file.

A complete final record without a newline remains readable. Append now adds the missing delimiter before its new record, avoiding concatenated JSON objects. Tests verify read failure does not alter original bytes and that appending to a valid unterminated final record round-trips through reload.

The session suite passed 20 race-enabled repetitions, 980 tests/subtests. FuzzSessionRecords passed a requested 60-second run (61.01 seconds elapsed), executing 3,395,073 inputs. Fresh full candidate race/coverage validation passed 802 tests/subtests, zero failures/skips. Session/runtime vet passed. Raw logs, coverage and digests are in harness-records.json; the incremental patch is harness-records.patch.

This is syntactic record validation, not complete DAG/schema validation. Safe tail removal with a preserved backup under the writer lease, future-version handling, selected branch persistence, migration/catalogue, all deferred/background persistence paths and Hand integration remain unfinished. No full M3.1 acceptance is claimed.
