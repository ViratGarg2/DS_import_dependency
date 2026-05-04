import pytest

from utils.parsing import extract_last_read_payload


@pytest.fixture
def cluster(cluster_factory):
    return cluster_factory(num_chunkservers=3, replication_factor=3)


def test_record_append_basic_flow(cluster) -> None:
    cluster.create_file("append-basic.txt")
    cluster.write_text("append-basic.txt", 0, "start-")
    cluster.append_text("append-basic.txt", "alpha")
    cluster.append_text("append-basic.txt", "beta")
    output = cluster.run_client(["read append-basic.txt 0 15"])
    payload = extract_last_read_payload(output)
    assert payload == "start-alphabeta", output


def test_record_append_rejects_payload_over_quarter_chunk(cluster) -> None:
    oversized_payload = "x" * 300000  # > 1/4th of 1MB chunk in this implementation
    cluster.create_file("append-too-large.txt")
    output = cluster.run_client(
        [
            f"append append-too-large.txt {oversized_payload}",
        ]
    )
    assert "Failed to append to file" in output, output
    assert "less than 1/4th of chunkSize" in output, output
