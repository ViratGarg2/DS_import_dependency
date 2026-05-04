import time

import pytest


@pytest.fixture
def cluster(cluster_factory):
    # 4 chunkservers with replication factor 3 gives one spare for re-replication.
    return cluster_factory(num_chunkservers=4, replication_factor=3)


@pytest.mark.slow
def test_re_replication_after_server_failure(cluster) -> None:
    cluster.create_file("re-replication.txt")
    cluster.write_text("re-replication.txt", 0, "recovery-test-data")

    chunk_handle = cluster.get_chunk_handle("re-replication.txt")
    initial_holders = cluster.ports_holding_chunk(chunk_handle)
    assert len(initial_holders) == 3, (
        f"Expected 3 initial replicas, got {len(initial_holders)} on {initial_holders}"
    )

    spare_ports = [p for p in cluster.chunk_ports if p not in initial_holders]
    assert len(spare_ports) == 1, f"Expected exactly one spare, found {spare_ports}"
    spare_port = spare_ports[0]

    primary_port = cluster.server_id_to_port(cluster.get_primary_server_id("re-replication.txt"))
    failed_port = next((p for p in initial_holders if p != primary_port), initial_holders[0])

    # Allow heartbeat propagation so master tracks the failed server's chunk ownership.
    time.sleep(3)
    cluster.stop_chunkserver(failed_port)

    cluster.wait_until(
        lambda: cluster.chunk_file_path(spare_port, chunk_handle).exists(),
        timeout=45,
        interval=1.0,
    )

    new_holders = cluster.ports_holding_chunk(chunk_handle)
    assert spare_port in new_holders, (
        f"Expected spare server {spare_port} to receive re-replicated chunk. "
        f"Current holders: {new_holders}"
    )
