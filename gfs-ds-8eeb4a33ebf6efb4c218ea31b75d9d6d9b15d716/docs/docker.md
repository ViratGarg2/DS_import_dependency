# Docker Multi-Node Test Guide

This setup runs GFS components in isolated containers on one machine:

- `master` on `50051`
- `chunk1` on `8080`
- `chunk2` on `8081` (mapped to container port `8080`)
- `client` container for interactive CLI

## 1. Start the cluster

```sh
docker compose up -d --build
```

## 2. Check container health/logs

```sh
docker compose ps
docker compose logs -f master chunk1 chunk2
```

## 3. Open the client CLI inside Docker

```sh
docker compose exec client /usr/local/bin/gfs-client --config /app/configs/docker/client-config.yml
```

Then run:

```text
create demo.txt
write demo.txt 0 hello
ls
read demo.txt 0 5
```

## 4. Fault-tolerance smoke test

In another terminal:

```sh
docker compose stop chunk2
```

Back in the client, run reads/writes again:

```text
read demo.txt 0 5
append demo.txt world
ls
```

Then restart:

```sh
docker compose start chunk2
docker compose logs -f chunk2
```

## 5. Cleanup

```sh
docker compose down
```

To also remove persisted volume data:

```sh
docker compose down -v
```

