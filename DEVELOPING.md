# Project Development

For general information about contributing changes, see the
[Contributor Guidelines](https://github.com/ditdotdev/.github/blob/master/CONTRIBUTING.md).

## How it Works

The proxy consists of two main pieces: the forwarder and the listener. The forwarder takes inputs in the form of
the docker API, invokes the appropriate dit method, and returns back docker specific objects. The listener is
responsible for listening on the appropriate Unix domain socket, routing requests,and marshaling / unmarshaling
data.

The command itself is just a wrapper around the internal methods, with command line arguments for specifying
things like the docker socket path and alternate ports.

## Building

To build the project, run `go build ./...`. This is equivalent to building `cmd/docker-volume-proxy/main.go`. This
will create a binary named `docker-volume-proxy` in the root of the directory.

## Testing

To test the project, run `go test ./...`. This will run all tests.

## Releasing

To release, create a tag and push it. This will build the resulting go binary for Linux (the runtime for the
dit-server container) and upload it as an artifact to the draft release. Release notes are maintained on each
push through the release drafter action.
