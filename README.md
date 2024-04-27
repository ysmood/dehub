# Overview

A lightweight and secure debugging lib for remote process. It's not a CLI tool, it's a library for you to build your own tool.

Features:

- Execute and attach to random CLI command on remote machine.
- Forward socks5 proxy on remote.
- Mount a remote directory to local with NFS.
- Uses the `golang.org/x/crypt/ssh` to establish secure connections.

```mermaid
flowchart LR
    H[Hub Server]
    M[Master client]
    S[Servant client]

    M ---> H
    S ---> H
```

Because Master and Servant uses public key to communicate, the Hub server can be a untrusted server.
