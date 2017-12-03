# Distributed PKI

remember to set $GOPATH to the root of this directory!
then, in src/distributepki: 
```
go get .
go build
```

## PBFT Setup

Setting up the PBFT cluster requires two configuration files to configure the
member nodes/prime the keystore for use. The cluster members are statically
assigned using a json file in this format:

```
{
    "nodes": [
        {
            "id": 1,
            "hostname": "<host1>:<port1>",
            "key": "<node 1 key>",
            "primary": true
        },
        {
            "id": 2,
            "hostname": "<host2>:<port2>",
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

Finally, to start up the cluster of `n` nodes, run `n` instances of
```
./distributepki --id i
```
## Client usage

To start up the client server, go to the `client/` directory and run `./client
--config ../distributepki/cluster.json`.

Currently, to look up a key initially inserted into the table, run the
following curl command:
```
curl -L http://localhost:<cluster node HTTP port>/<desired key>
```
Depending on the current status of the project, that may not work.

