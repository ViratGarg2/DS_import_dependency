import pytest


@pytest.fixture
def cluster(cluster_factory):
    return cluster_factory(num_chunkservers=3, replication_factor=3)


def test_initial_replication_factor_three(cluster) -> None:
    cluster.create_file("replicated.txt")
    cluster.write_text("replicated.txt", 0, "replicated-data")

    chunk_handle = cluster.get_chunk_handle("replicated.txt")
    holders = cluster.ports_holding_chunk(chunk_handle)

    assert len(holders) == 3, (
        f"Expected 3 replicas for chunk {chunk_handle}, found {len(holders)} on ports {holders}"
    )
