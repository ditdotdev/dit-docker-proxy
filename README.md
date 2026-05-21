# About this Project

This project provides a daemon that listens on a Docker plugin socket and forwards requests to
datadatdat-server. This enables the datadatdat-server API to remain independent of any docker-isms.

## Platform support

The daemon binds a **Unix domain socket** via `net.Listen("unix", ...)`. This works natively on
Linux and macOS, and on Windows 10 1803+ where the OS exposes `AF_UNIX` sockets — including under
Docker Desktop's Linux containers and recent WSL2-backed setups.

It does **not** use Windows named pipes (`\\.\pipe\...`), which is the transport that native
Windows Docker volume plugins traditionally expect. If you need named-pipe support, the listener
would need a `_windows.go` build-tag-gated variant that calls `github.com/Microsoft/go-winio`'s
`ListenPipe` — see [DEVELOPING.md](DEVELOPING.md).

## Contributing

This project follows the Datadatdat community best practices:

  * [Contributing](https://github.com/datadatdat/.github/blob/master/CONTRIBUTING.md)
  * [Code of Conduct](https://github.com/datadatdat/.github/blob/master/CODE_OF_CONDUCT.md)
  * [Community Support](https://github.com/datadatdat/.github/blob/master/SUPPORT.md)

It is maintained by the [Datadatdat community maintainers](https://github.com/datadatdat/.github/blob/master/MAINTAINERS.md)

For more information on how it works, and how to build and release new versions,
see the [Development Guidelines](DEVELOPING.md).
