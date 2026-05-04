from pathlib import Path

import pytest

from utils.parsing import extract_last_read_payload


@pytest.fixture
def cluster(cluster_factory):
    return cluster_factory(num_chunkservers=3, replication_factor=3)


def test_write_and_overwrite_readback(cluster) -> None:
    cluster.create_file("write-basic.txt")
    cluster.write_text("write-basic.txt", 0, "hello")
    cluster.write_text("write-basic.txt", 5, "_world")
    output = cluster.run_client(["read write-basic.txt 0 11"])
    payload = extract_last_read_payload(output)
    assert payload == "hello_world", output


@pytest.mark.slow
def test_write_file_size_exceeding_64mb(cluster, tmp_path: Path) -> None:
    # Write >64MB by sending two chunks of 33MB each to avoid one huge CLI command.
    part_size = 33 * 1024 * 1024
    file_a = tmp_path / "part_a.bin"
    file_b = tmp_path / "part_b.bin"
    file_a.write_bytes(b"A" * part_size)
    file_b.write_bytes(b"B" * part_size)

    cluster.create_file("write-large.txt")
    cluster.write_file("write-large.txt", 0, file_a)
    cluster.write_file("write-large.txt", part_size, file_b)
    output = cluster.run_client(
        [
            "read write-large.txt 0 8",
            f"read write-large.txt {part_size - 4} 8",
            f"read write-large.txt {(2 * part_size) - 8} 8",
        ],
        timeout=180,
    )

    # Validate three key read windows for >64MB data integrity.
    payloads = []
    for segment in output.split("Successfully read ")[1:]:
        payload = segment.split("bytes:\n", 1)[1].split("\ngfs>", 1)[0].rstrip("\n")
        payloads.append(payload)

    assert payloads[0] == "AAAAAAAA", output
    assert payloads[1] == "AAAABBBB", output
    assert payloads[2] == "BBBBBBBB", output
