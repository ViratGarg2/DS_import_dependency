"""Concurrent read / write / append integration tests for GFS."""

from __future__ import annotations

import threading
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest

from utils.parsing import extract_last_read_payload

NUM_WORKERS = 8
PAYLOAD = "ABCDEFGH"  # 8 visible ASCII bytes — easy to verify


@pytest.fixture
def cluster(cluster_factory):
    return cluster_factory(num_chunkservers=3, replication_factor=3)


# ── helpers ────────────────────────────────────────────────────────────────────

def _worker_payload(worker_id: int) -> str:
    """Deterministic 8-char payload per worker (char repeated 8 times)."""
    ch = chr(ord("A") + worker_id % 26)
    return ch * 8


def _fresh(cluster, filename: str) -> None:
    """Delete-then-create, ignoring 'not found' on delete."""
    cluster.run_client([f"delete {filename}"])
    cluster.run_client([f"create {filename}"])


# ── test 1: 8 concurrent writes to 8 different files ─────────────────────────

def test_concurrent_writes(cluster) -> None:
    files = [f"conc/write/f{i}" for i in range(NUM_WORKERS)]
    for fn in files:
        _fresh(cluster, fn)

    errors: list[str] = []
    lock = threading.Lock()

    def do_write(i: int) -> None:
        payload = _worker_payload(i)
        try:
            cluster.write_text(files[i], 0, payload)
        except AssertionError as exc:
            with lock:
                errors.append(f"worker {i}: {exc}")

    with ThreadPoolExecutor(max_workers=NUM_WORKERS) as pool:
        futures = [pool.submit(do_write, i) for i in range(NUM_WORKERS)]
        for f in as_completed(futures):
            f.result()  # re-raise unexpected exceptions

    assert not errors, "Concurrent writes failed:\n" + "\n".join(errors)


# ── test 2: 8 concurrent reads with integrity check ──────────────────────────

def test_concurrent_reads(cluster) -> None:
    """Reads back the files written in the previous test — but as a self-
    contained test we re-create and re-write them first."""
    files = [f"conc/read/f{i}" for i in range(NUM_WORKERS)]
    for i, fn in enumerate(files):
        _fresh(cluster, fn)
        cluster.write_text(fn, 0, _worker_payload(i))

    errors: list[str] = []
    lock = threading.Lock()

    def do_read(i: int) -> None:
        fn = files[i]
        expected = _worker_payload(i)
        try:
            output = cluster.run_client([f"read {fn} 0 {len(expected)}"])
            payload = extract_last_read_payload(output)
            if payload != expected:
                with lock:
                    errors.append(
                        f"worker {i}: expected {expected!r}, got {payload!r}"
                    )
        except AssertionError as exc:
            with lock:
                errors.append(f"worker {i}: {exc}")

    with ThreadPoolExecutor(max_workers=NUM_WORKERS) as pool:
        futures = [pool.submit(do_read, i) for i in range(NUM_WORKERS)]
        for f in as_completed(futures):
            f.result()

    assert not errors, "Concurrent reads failed:\n" + "\n".join(errors)


# ── test 3: 4 concurrent writers + 4 concurrent readers (different files) ────

def test_mixed_read_write(cluster) -> None:
    write_files = [f"conc/mixed/w{i}" for i in range(4)]
    read_files = [f"conc/mixed/r{i}" for i in range(4)]

    # Pre-create read files with known content.
    for i, fn in enumerate(read_files):
        _fresh(cluster, fn)
        cluster.write_text(fn, 0, _worker_payload(i))

    # Pre-create (empty) write targets.
    for fn in write_files:
        _fresh(cluster, fn)

    errors: list[str] = []
    lock = threading.Lock()

    def do_write(i: int) -> None:
        try:
            cluster.write_text(write_files[i], 0, _worker_payload(i + 10))
        except AssertionError as exc:
            with lock:
                errors.append(f"writer {i}: {exc}")

    def do_read(i: int) -> None:
        fn = read_files[i]
        expected = _worker_payload(i)
        try:
            output = cluster.run_client([f"read {fn} 0 {len(expected)}"])
            payload = extract_last_read_payload(output)
            if payload != expected:
                with lock:
                    errors.append(
                        f"reader {i}: expected {expected!r}, got {payload!r}"
                    )
        except AssertionError as exc:
            with lock:
                errors.append(f"reader {i}: {exc}")

    with ThreadPoolExecutor(max_workers=8) as pool:
        futures = (
            [pool.submit(do_write, i) for i in range(4)]
            + [pool.submit(do_read, i) for i in range(4)]
        )
        for f in as_completed(futures):
            f.result()

    assert not errors, "Mixed read+write failed:\n" + "\n".join(errors)


# ── test 4: 8 concurrent appenders to the same file ──────────────────────────

def test_concurrent_appends(cluster) -> None:
    fn = "conc/append/shared"
    _fresh(cluster, fn)

    errors: list[str] = []
    lock = threading.Lock()

    def do_append(i: int) -> None:
        payload = _worker_payload(i)
        try:
            cluster.append_text(fn, payload)
        except AssertionError as exc:
            with lock:
                errors.append(f"appender {i}: {exc}")

    with ThreadPoolExecutor(max_workers=NUM_WORKERS) as pool:
        futures = [pool.submit(do_append, i) for i in range(NUM_WORKERS)]
        for f in as_completed(futures):
            f.result()

    assert not errors, "Concurrent appends failed:\n" + "\n".join(errors)
