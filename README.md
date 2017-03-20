# Aliases Service

This service is used generate aliases for internal identifiers.

The primary use case is supplying an internal identifier and generating a random alias that is intended to be user facing.

Alias generation is idempotent, that is, identifiers that have been seen before will return the same alias. Similar to one-way hashes, the API does not support taking an alias and returning the associated identifer. Thus in order to retrieve the alias, the client must have the identifier to begin with.

Features:

- Random character-based IDs or UUIDs.
- Adding a prefix to the random portion of the alias.
- Specifying a set of valid characters for generation.
- Sequence generation

## Example

Send a POST request to the root with a newline delimited set of identifiers to generate aliases for. The order of the aliases will match the order of the identifiers.

```
curl -XPOST localhost:8080 --data-binary "39323289
3203233
9909382"
```

Response:

```
zzvi7hvs
zwk1b182
tvjzjpez
```

## Dependencies

- Redis

## Service

### Options

All of these options have defaults. To view, run `aliases -help`.

**Redis**
- `redis` - The address to the Redis database.
- `redis.db` - The specific Redis database to use.
- `redis.pass` - A password to authenticate with Redis for establishing connections.

**HTTP**
- `http` - The bind address for the service.
- `http.tls.cert` - The TLS certificate file name.
- `http.tls.key` - The TLS key file name.

**Alias**
- `type` - The type of alias to generate, either `chars` for random characters or `uuid` for a UUID.
- `prefix` - A fixed prefix to prepend to generated aliases.
- `chars.minlen` - The minimum length of a `chars`-based generated alias.
- `chars.valid` - A sequence of valid characters to use when generating a `chars`-based alias.

## Usage

Go to the [releases](https://github.com/chop-dbhi/aliases/releases) page and download the build for your platform. Unpack it and run it.

```
aliases
```

A Docker image is also available.

```
docker run --rm -it -p 8080:8080 dbhi/aliases
```
