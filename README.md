# Distributed PKI

remember to set $GOPATH to the root of this directory!
then, in src/distributepki: 
```
go get .
go build
```

## Architecture (updated 12/2/17)

Currently, the architecture of the project is closely tied to the PBFT backing
algorithm. A `client` server communicates with the PBFT primary over RPC, and
each replica communicates with its peers over RPC as well. In picture form:

```
+--------+             +----------------------+
| Client | <-- RPC --> | KeyNode +----------+ |
+--------+             |    ^    | Keystore | |             +----------+
                       |    |    +----------+ |             |    ...   |
                       +----v-----------------+             +----------+
                       |       PBFTNode       | <-- RPC --> | PBFTNode |
                       +----------------------+             +----------+
```

## PBFT Setup

Setting up the PBFT cluster requires two configuration files to configure the
member nodes/prime the keystore for use. The cluster members are statically
assigned using a json file in this format:

TODO: rename rpcport to something that makes sense
```
{
    "endpoint": "pbft",
    "nodes": [
        {
            "id": 1,
            "hostname": "<host1>",
            "port": <port>,
            "rpcport": <external port>,
            "key": "<node 1 key>",
            "primary": true
        },
        {
            "id": 2,
            "hostname": "<host2>",
            "port": <port2>,
            "rpcport": <external port2>,
            "key": "<node 2 key>",
            "primary": false
        },
        ...
    ]
}
```

There should only be one primary node, and each node must have their own PGP
key pair, the public one specified in the cluster configuration. In addition,
any nodes that are authorized to add new public keys for their domains should
be included in a json file to initialize the key store:

```
[
    {
        "alias": "google.com",
        "key": "<key>"
    },
    ...
]
```

Finally, to start up the cluster of `n` nodes acording to `cluster.json`, run
 `./distributepki -cluster`.

## Debugging
If you enable debugging on your cluster (on by default right now), you can
you can also run a debugging REPL with just `./distributepki -debug`. The
REPL supports the following commands:
  * `put <id> <alias> <key>`   tells the node to commit a put operation
  * `get <id> <alias>`         tells the node to read
  * `down <id>`                takes down the node with the specified id,
                             until `up <id>` is called
  * `up <id>`                  brings the node with the specified id back up
  * `exit`                     quits the repl

## Client usage

To start up the client server, go to the `client/` directory and run `./client
--config ../distributepki/cluster.json`.

Currently, to look up a key initially inserted into the table, run the
following curl command:
```
curl -L http://localhost:<cluster node HTTP port>?name=<desired alias>
```
or POST to `http://localhost:<HTTP port>?name=<desired key>` with the request
body as the value you want to set the key.

Depending on the current status of the project, that may not work.

## TODO:

*this* means we're working on it

### core functionality
 - [X] Actually sign and verify reads
 - [X] Catch up nodes properly (piggybacking entries on heartbeats
       and re-preparing non-checkpointed entries)
 - [X] View changes on client request timeout & on heartbeat timeout
 - [ ] *Checkpointing* (JL)
    * limit sequence nums properly

### not core, but also important
 - [ ] Check for resource leaks
 - [X] tests?? l0l
 - [X] moar tests
 - [ ] Reuse RPC connections

# Implementation details
We mostly follow the design sketched out in the original PBFT paper, with a couple
of small changes to the implementation:

### Heartbeats
According to the PBFT paper, nodes start a timer when they hear of a client request.
If the timer expires without having committed/executed the request, that node initiates
a view change. The downside to this is that if a node is compromised or goes down, 
we don't discover it until the next client request, impacting percieved liveness.

So we introduce heartbeats. If a node does not hear a heartbeat from the view's
primary for a while, it initiates a view change.

### Node recovery
The PBFT paper is a bit vague on how it handles retransmissions and node recovery
apart from view changes (which are very expensive), and also admits to not having
fully implemented view changes & retransmissions. We take a page from Raft's book,
and have all nodes piggyback state information onto heartbeat messages. A node's
response to the heartbeat can be its own most recently committed sequence number,
so the primary knows what preprepares to rebroadcast to the node.

### client/demo: colin
 - [ ] CRX
 - [ ] spin up mock cluster/email servers

