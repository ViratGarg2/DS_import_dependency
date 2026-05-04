import os
import re
import socket
import subprocess
import time
from dataclasses import dataclass, field
from pathlib import Path


ANSI_ESCAPE_RE = re.compile(r"\x1B\[[0-?]*[ -/]*[@-~]")
CHUNK_HANDLE_RE = re.compile(r"Handle:\s*([0-9a-fA-F-]{36})")
PRIMARY_RE = re.compile(r"Primary:\s*([^\s]+)")


def strip_ansi(text: str) -> str:
    return ANSI_ESCAPE_RE.sub("", text)


def get_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@dataclass
class GFSCluster:
    repo_root: Path
    binaries: dict[str, Path]
    root_dir: Path
    num_chunkservers: int = 3
    replication_factor: int = 3
    lease_request_interval: int = 2
    health_check_interval: int = 1
    health_timeout: int = 2
    max_failures: int = 1

    master_port: int = field(default_factory=get_free_port)
    chunk_ports: list[int] = field(default_factory=list)
    processes: dict[str, subprocess.Popen] = field(default_factory=dict)

    def __post_init__(self) -> None:
        if not self.chunk_ports:
            self.chunk_ports = [get_free_port() for _ in range(self.num_chunkservers)]
        self.root_dir.mkdir(parents=True, exist_ok=True)
        self.logs_dir = self.root_dir / "logs"
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        self.storage_dir = self.root_dir / "storage"
        self.storage_dir.mkdir(parents=True, exist_ok=True)
        self.master_storage_dir = self.storage_dir / "master"
        self.master_storage_dir.mkdir(parents=True, exist_ok=True)
        self.chunk_storage_dir = self.storage_dir / "chunks"
        self.chunk_storage_dir.mkdir(parents=True, exist_ok=True)
        self.config_dir = self.root_dir / "configs"
        self.config_dir.mkdir(parents=True, exist_ok=True)
        self.master_config = self.config_dir / "master.yml"
        self.chunk_config = self.config_dir / "chunkserver.yml"
        self.client_config = self.config_dir / "client.yml"
        self._write_configs()

    @property
    def master_address(self) -> str:
        return f"127.0.0.1:{self.master_port}"

    def server_id_for_port(self, port: int) -> str:
        return f"server-127-0-0-1-{port}"

    def chunk_file_path(self, port: int, chunk_handle: str) -> Path:
        server_id = self.server_id_for_port(port)
        return self.chunk_storage_dir / server_id / f"{chunk_handle}.chunk"

    def _write_configs(self) -> None:
        metadata_path = self.master_storage_dir / "metadata.json"
        op_log_path = self.master_storage_dir / "operation-log.json"
        self.master_config.write_text(
            f"""---
chunk:
  size: 1048576
  naming_pattern: "chunk_{{id}}"
  checksum_algorithm: "sha256"
  verify_on_read: true

chunk_server:
  max_chunks: 1000
  storage_path: "{self.chunk_storage_dir.as_posix()}"

deletion:
  g_c_interval: 180
  retention_period: 300
  g_c_delete_batch_size: 100
  trash_dir_prefix: "/.trash/"

health:
  check_interval: {self.health_check_interval}
  timeout: {self.health_timeout}
  max_failures: {self.max_failures}

metadata:
  database:
    type: "json"
    path: "{metadata_path.as_posix()}"
    backup_interval: 5
  max_filename_length: 255
  max_directory_depth: 64

lease:
  lease_timeout: 5

operation_log:
  path: "{op_log_path.as_posix()}"

replication:
  factor: {self.replication_factor}
  timeout: 30

server:
  host: "127.0.0.1"
  port: {self.master_port}
  max_connections: 100
  connection_timeout: 30
  max_request_size: 104857600
  thread_pool_size: 50
""",
            encoding="utf-8",
        )

        self.chunk_config.write_text(
            f"""server:
  master_address: "{self.master_address}"
  data_dir: "{self.chunk_storage_dir.as_posix()}"
  heartbeat_interval: 1
  lease_timeout: 5
  lease_request_interval: {self.lease_request_interval}

storage:
  max_chunk_size: 1048576
  buffer_size: 65536
  flush_interval: 5

operation:
  read_timeout: 30
  write_timeout: 60
  retry_attempts: 3
  retry_delay: 1
""",
            encoding="utf-8",
        )

        self.client_config.write_text(
            f"""connection:
  master:
    host: "127.0.0.1"
    port: {self.master_port}
    timeout: 30
  max_retries: 3
  retry_interval: 1
  request_timeout: 60

cache:
  chunk_location:
    enabled: true
    size: 1000
    ttl: 5
  metadata:
    enabled: true
    size: 10000
    ttl: 10

operation:
  chunk:
    read_size: 1048576
    write_size: 1048576
    verify_writes: true
  retries:
    max_attempts: 3
    backoff_base: 2
  timeouts:
    read: 30
    write: 60
    delete: 30

logging:
  level: "info"
  format: "json"
  directory: "{(self.root_dir / 'client-logs').as_posix()}"
  max_size: 100
  max_files: 5

monitoring:
  enabled: false
  update_interval: 60
  metrics_port: 9090
""",
            encoding="utf-8",
        )

    def _start_process(self, name: str, cmd: list[str]) -> subprocess.Popen:
        log_path = self.logs_dir / f"{name}.log"
        log_file = open(log_path, "a", encoding="utf-8")
        env = os.environ.copy()
        env["GOCACHE"] = env.get("GOCACHE", "/tmp/gocache")
        proc = subprocess.Popen(
            cmd,
            cwd=self.repo_root,
            stdout=log_file,
            stderr=subprocess.STDOUT,
            env=env,
        )
        self.processes[name] = proc
        return proc

    def _wait_for_port(self, host: str, port: int, timeout: float = 20.0) -> None:
        deadline = time.time() + timeout
        while time.time() < deadline:
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
                s.settimeout(0.2)
                try:
                    s.connect((host, port))
                    return
                except OSError:
                    time.sleep(0.2)
        raise TimeoutError(f"Timed out waiting for {host}:{port}")

    def start(self) -> None:
        self._start_process(
            "master",
            [
                str(self.binaries["master"]),
                "--config",
                str(self.master_config),
            ],
        )
        self._wait_for_port("127.0.0.1", self.master_port)

        for port in self.chunk_ports:
            self.start_chunkserver(port)

        self.wait_until(
            lambda: "Welcome to GFS Client!" in self.run_client([]),
            timeout=20,
            interval=0.5,
        )

    def start_chunkserver(self, port: int) -> None:
        name = f"chunk-{port}"
        self._start_process(
            name,
            [
                str(self.binaries["chunkserver"]),
                "--config",
                str(self.chunk_config),
                "--host",
                "127.0.0.1",
                "--listen-host",
                "127.0.0.1",
                "--port",
                str(port),
            ],
        )
        self._wait_for_port("127.0.0.1", port)

    def stop_chunkserver(self, port: int) -> None:
        name = f"chunk-{port}"
        proc = self.processes.get(name)
        if not proc:
            return
        if proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=5)

    def stop(self) -> None:
        for name in sorted(self.processes.keys(), reverse=True):
            proc = self.processes[name]
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait(timeout=5)

    def run_client(self, commands: list[str], timeout: int = 120) -> str:
        client_cmd = [
            str(self.binaries["client"]),
            "--config",
            str(self.client_config),
        ]
        payload = "\n".join(commands + ["exit"]) + "\n"
        proc = subprocess.run(
            client_cmd,
            cwd=self.repo_root,
            text=True,
            input=payload,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=timeout,
            check=False,
            env={**os.environ, "GOCACHE": os.environ.get("GOCACHE", "/tmp/gocache")},
        )
        return strip_ansi(proc.stdout)

    def create_file(self, filename: str) -> str:
        output = self.run_client([f"create {filename}"])
        if "File created successfully" in output or "file already exists" in output:
            return output
        raise AssertionError(f"Create failed for {filename}.\nOutput:\n{output}")

    def write_text(self, filename: str, offset: int, content: str, retries: int = 15, delay: float = 0.5) -> str:
        last_output = ""
        for _ in range(retries):
            output = self.run_client([f"write {filename} {offset} {content}"])
            last_output = output
            if "Successfully wrote" in output:
                return output
            if (
                "server is not primary for chunk" in output
                or "failed to forward to" in output
                or "connection refused" in output
                or "rpc error: code = Unavailable" in output
                or "connection reset by peer" in output
            ):
                time.sleep(delay)
                continue
            break
        raise AssertionError(f"Write failed for {filename}.\nOutput:\n{last_output}")

    def write_file(self, filename: str, offset: int, file_path: Path, retries: int = 15, delay: float = 0.5) -> str:
        last_output = ""
        for _ in range(retries):
            output = self.run_client([f"writefile {filename} {offset} {file_path}"], timeout=240)
            last_output = output
            if "Successfully wrote" in output:
                return output
            if (
                "server is not primary for chunk" in output
                or "failed to forward to" in output
                or "connection refused" in output
                or "rpc error: code = Unavailable" in output
                or "connection reset by peer" in output
            ):
                time.sleep(delay)
                continue
            break
        raise AssertionError(f"Writefile failed for {filename}.\nOutput:\n{last_output}")

    def append_text(self, filename: str, content: str, retries: int = 30, delay: float = 0.5) -> str:
        last_output = ""
        for _ in range(retries):
            output = self.run_client([f"append {filename} {content}"])
            last_output = output
            if "Successfully appended data at offset" in output:
                return output
            if (
                "server is not primary for chunk" in output
                or "failed to forward to" in output
                or "connection refused" in output
                or "rpc error: code = Unavailable" in output
                or "connection reset by peer" in output
            ):
                time.sleep(delay)
                continue
            break
        raise AssertionError(f"Append failed for {filename}.\nOutput:\n{last_output}")

    def get_chunk_handle(self, filename: str, index: int = 0) -> str:
        output = self.run_client([f"chunks {filename} {index} {index}"])
        match = CHUNK_HANDLE_RE.search(output)
        if not match:
            raise AssertionError(f"Could not parse chunk handle.\nOutput:\n{output}")
        return match.group(1)

    def get_primary_server_id(self, filename: str, index: int = 0) -> str:
        output = self.run_client([f"chunks {filename} {index} {index}"])
        match = PRIMARY_RE.search(output)
        if not match:
            raise AssertionError(f"Could not parse primary server id.\nOutput:\n{output}")
        return match.group(1)

    def server_id_to_port(self, server_id: str) -> int:
        try:
            return int(server_id.rsplit("-", 1)[1])
        except (IndexError, ValueError) as exc:
            raise AssertionError(f"Unexpected server_id format: {server_id}") from exc

    def ports_holding_chunk(self, chunk_handle: str) -> list[int]:
        holders = []
        for port in self.chunk_ports:
            if self.chunk_file_path(port, chunk_handle).exists():
                holders.append(port)
        return holders

    def wait_until(self, predicate, timeout: float = 20.0, interval: float = 0.5) -> None:
        deadline = time.time() + timeout
        while time.time() < deadline:
            if predicate():
                return
            time.sleep(interval)
        raise TimeoutError("Condition was not met before timeout.")
