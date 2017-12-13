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

## Authority server

Make sure to spin up the mock authority server before using the cluster, which
signs off on initial domain<=>key pairings.  Go to `mock_authority`,
`go get && go build`, and `./mock_authority`.

## PBFT Setup

Setting up the PBFT cluster requires two configuration files to configure the
member nodes/prime the keystore for use. The cluster members are statically
assigned using a json file in this format:

```
{
    "endpoint": "pbft",
    "nodes": [
        {
            "id": 1,
            "hostname": "<host1>",
            "port": <port for internal messages>,
            "clientport": <external port>,
            "publickeyfile": <location of public pgp key>,
            "privatekeyfile": <location of secret pgp key>,
            "passphrasefile": <location of secret for key>,
        },
        {
            "id": 2,
            "hostname": "<host2>",
            "port": <port2>,
            "clientport": <external port2>,
            "publickeyfile": <location of public pgp key>,
            "privatekeyfile": <location of secret pgp key>,
            "passphrasefile": <location of secret for key>,
        },
        ...
    ]
}
```

Each node must have their own PGP key pair, the public one specified in the
cluster configuration. In addition, any nodes that are authorized to add new
public keys for their domains should be included in a json file to initialize
the key store:

```
[
    {
        "alias": "google.com",
        "key": "<key>"
    },
    ...
]
```

To build, run `go get && go build` in `/distributepki`.

To start up a local cluster of `n` nodes acording to `cluster.json`, run
 `./distributepki -cluster`. To start one machine at a time, run `./distributepki -id <id>`.
You can also configure which config file to use using `-config <cluster config file>`.
Make sure the auth server is running!

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

## Testing

Run `go test` to test the cluster. Make sure the auth server is running!

## Client usage

Currently, to look up a key initially inserted into the table, our cluster
uses the following HTTP API:

```
Updates: PUT /:
  request body: {
    Alias:     <name to update>,
    Key:       <key to issue>,
    Timestamp: <time of operation>,
    Signature: <signature on operation with previous key>
  }
Creates: POST /:
  request body: {
    Alias:     <name to update>,
    Key:       <key to issue>,
    Timestamp: <time of operation>,
    Signature: <signature on operation by an authority>
  }
Lookups:  GET /?name=<desired alias>
```

So you can run `curl -L http://<cluster host>:<cluster node HTTP port>?name=<desired alias>`
to perform lookups,
or PUT/POST to `http://<cluster host>:<HTTP port>?name=<desired key>` with the request
body as defined above.

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

## TODO:
 - [ ] moar tests
 - [ ] Reuse RPC connections
