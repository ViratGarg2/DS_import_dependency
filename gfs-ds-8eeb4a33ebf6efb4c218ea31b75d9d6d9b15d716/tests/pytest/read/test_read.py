from pathlib import Path

import pytest

from utils.parsing import extract_last_read_payload


CHUNK_SIZE = 1 * 1024 * 1024


@pytest.fixture
def cluster(cluster_factory):
    return cluster_factory(num_chunkservers=3, replication_factor=3)


def test_read_across_chunk_boundary(cluster, tmp_path: Path) -> None:
    # Deterministic 2MB payload of visible ASCII for stable output parsing.
    data = (b"0123456789ABCDEF" * (2 * CHUNK_SIZE // 16))[: 2 * CHUNK_SIZE]
    local_file = tmp_path / "read-boundary.bin"
    local_file.write_bytes(data)

    offset = CHUNK_SIZE - 8
    length = 16
    expected = data[offset : offset + length].decode("ascii")

    cluster.create_file("read-boundary.txt")
    cluster.write_file("read-boundary.txt", 0, local_file)
    output = cluster.run_client([f"read read-boundary.txt {offset} {length}"], timeout=180)
    payload = extract_last_read_payload(output)
    assert payload == expected, output


def test_read_invalid_length_returns_error(cluster) -> None:
    cluster.create_file("read-invalid.txt")
    cluster.write_text("read-invalid.txt", 0, "abc")
    output = cluster.run_client(["read read-invalid.txt 0 0"])
    assert "Failed to read file" in output, output
