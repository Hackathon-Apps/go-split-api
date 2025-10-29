# Split Bill API

### Local stand:
```bash
make
./go-split-api
```
Do not forget to create `split.toml` file from example.

##### For local development
Goose setup
```bash
export GOOSE_DRIVER=postgres
export GOOSE_MIGRATION_DIR=./db/migrations
# get env variable from ./configs/split.toml file
export GOOSE_DBSTRING=postgres://username:password@db_host:db_port/db_name
goose up
```

See API call examples in `./api.http` file.