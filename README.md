# Dehub

The side effects are always the hardest part of system design. How about we have a project to abstract all the side effects away and make other parts of the system like pure functions. Or at least help to monitor or record all the side effects.

The function of this project is pretty simple, proxy all IO related dependencies of a system so that all the micro-services inside the system can be designed as pure functions.

An example scenario looks like below:

The dependency topology of Client, Service 1, 2, 3, 4, 5, 6:

```text
C -> (S1, S2) -> (S3, S4)
         |           ^
         v           |
      (S5, S6) ------+

# a pair of braces is a cluster
# S3 and S4 is the database
```

With dehub the graph will become:

```text
C -> dehub <--+-- S1
              +-- S2
              +-- S3
              +-- S4
              +-- S5
              +-- S6
```

All services connect to dehub and consume the events they care and map the events to data then store them back to dehub.

With this architecture, we can do a lot of things, graceful restart services without waiting for all connections are closed, monitor and replay the traffic, deep trace request flow,
load test selected services, no need to set load balancer for each cluster, etc.

For example, test a module with all its dependencies without having to mock data.

```text
local C -> dehub <-- S5
             ^
local S1 ----+
local S3 ----+
```

You can config dehub and your local C, S1, S3, and remote S5 to handle the input.

## Limitations

Currently, there's no easy way to proxy third-party libs or private protocols. So we can't track the content of them, we can only log the event into dehub. But normally a good library will give the interface to let user choose how to handle the side effect.

