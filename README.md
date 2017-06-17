# hydra-oracle-plugin

This plugin provides OracleDB (11g, 12g) connectivity for [ORY Hydra](https://github.com/ory/hydra). It builds
only on linux.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Installation](#installation)
- [Development](#development)
- [Usage](#usage)
  - [DSN](#dsn)
  - [Schema Creation & Migration](#schema-creation-&-migration)
  - [Running with ORY Hydra](#running-with-ory-hydra)
- [Todo](#todo)
  - [ORA Version](#ora-version)
  - [Schema Migration](#schema-migration)
- [Known Issues](#known-issues)
  - [Pagination not working for policy fetching](#pagination-not-working-for-policy-fetching)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Installation

This repo uses docker-compose for easy development and compilation as Linux, GCC and Oracle SDK need to be
available and set up. To run this environment, do:

```
# export ORACLE_DSN=... - to learn more about the oracle DSN layout, check in the sections below.
docker-compose up --build -d
```

To run tests, do:

```
docker exec -t -i hydraoracleplugin_plugin_1 /bin/bash
go test -v .
```

To build the plugin, do

```
docker exec -t -i hydraoracleplugin_plugin_1 /bin/bash
go build --buildmode=plugin -o ./build/plugin-ora.so ./*.go
```

and look on your host file system for `./build/plugin-ora.so`.

## Development

You are encouraged to use the docker container for developing this plugin. The root directory is mounted as a volume
in the `./dev` directory in the container. Running tests looks, for example, like this:

```
docker exec -t -i hydraoracleplugin_plugin_1 /bin/bash
cd dev
go test -v .
```

## Usage

### DSN

The DSN is layouted as follows:

```
<user>/<password>@<host>:<port>/<database>/<schema>
```

for example:

```
user/password@somehost.com:1521/ORCL/user
```

### Schema Creation & Migration

```
docker exec -t -i hydraoracleplugin_plugin_1 /bin/bash
hydra-oracle-plugin migrate <DSN>
# for example (if -e ORACLE_DSM=.. is set in the docker exec command):
# hydra-oracle-plugin migrate $ORACLE_DSN
```

### Running with ORY Hydra

On your host system, do:

```
docker build -t hydra-ora-plugin -f Dockerfile-hydra .
docker run -p 4444:4444 -e SYSTEM_SECRET=someverysecuresecret -e DATABASE_URL=... -e DATABASE_PLUGIN=/go/src/github.com/ory/hydra/plugin-ora.so  -e ISSUER=https://localhost:4444/ hydra-ora-plugin
```

## Todo

### ORA Version

Currently, [ora is fetched](./Dockerfile-hydra) with `go get gopkg.in/rana/ora.v4`. Instead, this should be done
with a locked version.

### Schema Migration

No schema migration is implemented at the moment.

## Known Issues

### Pagination not working for policy fetching

Since oracle doesn't support an easy way to do `LIMIT` or `OFFSET`, pagination has been disabled for policies. Instead,
all policies are fetched from the backend and a slice is used to return policies in range of limit and offset.