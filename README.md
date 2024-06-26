# Overview

A lightweight and secure debugging lib for remote process.
Especially useful for debugging a service that you can deploy code to but you don't have direct access to,
such as as k8s pod, github workflow, etc.

Features:

- Execute and attach to random CLI command on remote machine.
- Forward socks5 proxy on remote.
- Mount a remote directory to local with NFS.
- Uses the `golang.org/x/crypt/ssh` to establish secure connections.
- Hub server can be an endpoint of a http server.
- Servant can run behind a firewall.

```mermaid
flowchart LR
    M[Master Client]
    S[Servant Client]

    subgraph Cluster
        H1[Hub Server 01]
        H2[Hub Server 02]
    end

    S ---> H2
    M --> H1
    H1 -- relay --> H2
```

Because Master and Servant uses public key to communicate, the Hub server can be a untrusted server.
