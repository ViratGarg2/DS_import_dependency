# Running Instructions

The entry points for all the components (master, clients and chunkservers) is present within `./cmd` directory.

## Running the Master

Only one master is allowed to run. By default, it uses the 50051 port.

```sh
cd cmd/master
go run main.go
```

Optional:

```sh
go run main.go --config <path-to-master-config>
```

## Running Chunk Servers

Open different terminal instances per chunk server. Replicatin kicks in when there are >1 chunk servers running.

```sh
cd cmd/chunkserver
go run main.go --port <port> --host <advertised-host-or-ip> --listen-host <bind-host>
```

Examples:

```sh
# Same machine
go run main.go --port 8080 --host localhost --listen-host 0.0.0.0

# Different machine
go run main.go --port 8080 --host 10.0.0.21 --listen-host 0.0.0.0
```

## Running Clients

Open different terminal instances per client.

```sh
cd cmd/client
go run main.go
```

Optional:

```sh
go run main.go --config <path-to-client-config>
```

## Configurations

All configurations are modifiable from within the `configs` directory.

For containerized multi-node testing on one machine, see [docs/docker.md](docker.md).
