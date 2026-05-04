# Pytest Integration Suite (GFS MVP)

This suite provides feature-separated integration tests for:

- `write/`
- `read/`
- `replication/`
- `record_append/`
- `re_replication/`
- `stale_replica_handling/`

## What it tests

- Write/read correctness
- Overwrite behavior
- Read across chunk boundaries
- Large-file edge case (`>64MB`)
- Initial replication factor behavior
- Record append success and oversized append rejection
- Re-replication after a replica failure (with spare server)
- Stale replica cleanup after node rejoin

## How to run

From repo root:

```bash
python3 -m pytest -q tests/pytest
```

Run a single feature directory:

```bash
python3 -m pytest -q tests/pytest/write
```

## Notes

- Tests spin up real master/chunkserver/client processes with isolated temp configs and storage.
- Ports are auto-selected per test run.
- Some tests are marked `slow` in `tests/pytest/pytest.ini`.
