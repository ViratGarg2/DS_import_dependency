import time

import pytest


@pytest.fixture
def cluster(cluster_factory):
    return cluster_factory(num_chunkservers=3, replication_factor=3, lease_request_interval=2)


@pytest.mark.slow
def test_stale_replica_is_deleted_on_rejoin(cluster) -> None:
    cluster.create_file("stale-replica.txt")
    cluster.write_text("stale-replica.txt", 0, "base")

    chunk_handle = cluster.get_chunk_handle("stale-replica.txt")
    primary_server_id = cluster.get_primary_server_id("stale-replica.txt")
    primary_port = cluster.server_id_to_port(primary_server_id)

    holders = cluster.ports_holding_chunk(chunk_handle)
    assert len(holders) >= 2, f"Need at least 2 replicas, got holders: {holders}"

    target_port = next(p for p in holders if p != primary_port)
    stale_file_path = cluster.chunk_file_path(target_port, chunk_handle)
    assert stale_file_path.exists(), f"Expected stale candidate file to exist: {stale_file_path}"

    cluster.stop_chunkserver(target_port)
    # Let lease renewals advance versions on active replicas while this server is down.
    time.sleep(10)

    cluster.start_chunkserver(target_port)

    cluster.wait_until(
        lambda: not stale_file_path.exists(),
        timeout=30,
        interval=1.0,
    )
