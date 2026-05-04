import subprocess
from pathlib import Path

import pytest

from utils.cluster import GFSCluster


@pytest.fixture(scope="session")
def repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


@pytest.fixture(scope="session")
def gfs_binaries(tmp_path_factory: pytest.TempPathFactory, repo_root: Path) -> dict[str, Path]:
    bin_dir = tmp_path_factory.mktemp("gfs_bins")
    binaries = {
        "master": bin_dir / "gfs-master",
        "chunkserver": bin_dir / "gfs-chunkserver",
        "client": bin_dir / "gfs-client",
    }

    subprocess.run(
        ["go", "build", "-o", str(binaries["master"]), "./cmd/master"],
        cwd=repo_root,
        check=True,
    )
    subprocess.run(
        ["go", "build", "-o", str(binaries["chunkserver"]), "./cmd/chunkserver"],
        cwd=repo_root,
        check=True,
    )
    subprocess.run(
        ["go", "build", "-o", str(binaries["client"]), "./cmd/client"],
        cwd=repo_root,
        check=True,
    )

    return binaries


@pytest.fixture
def cluster_factory(tmp_path: Path, repo_root: Path, gfs_binaries: dict[str, Path]):
    clusters: list[GFSCluster] = []

    def _create(
        *,
        num_chunkservers: int = 3,
        replication_factor: int = 3,
        lease_request_interval: int = 2,
        health_check_interval: int = 1,
        health_timeout: int = 2,
        max_failures: int = 1,
    ) -> GFSCluster:
        cluster = GFSCluster(
            repo_root=repo_root,
            binaries=gfs_binaries,
            root_dir=tmp_path / f"cluster-{len(clusters)}",
            num_chunkservers=num_chunkservers,
            replication_factor=replication_factor,
            lease_request_interval=lease_request_interval,
            health_check_interval=health_check_interval,
            health_timeout=health_timeout,
            max_failures=max_failures,
        )
        cluster.start()
        clusters.append(cluster)
        return cluster

    yield _create

    for cluster in reversed(clusters):
        cluster.stop()
