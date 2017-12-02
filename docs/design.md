# PKI on Stellar: design doc
JL, colin, & syd for CS244b

We propose a distributed public-key database implemented on top of Stellar's consensus protocol. 

From a security standpoint, we want to provide:
 - Non-equivocation: The cluster cannot report two different sets of keys for the same user. Consistency in consensus implies non-equivocation. 
 - Continuity: the cluster cannot *change* a user's key set without a proposal signed by one of the user's existing keys. When validating a new proposal to change a user's keyset, the node does an additional check to ensure it was signed by one of the user's original keys.
 - Privacy/information hiding
    - The database reveals no information about the users... how??? TODO
    - ideas: maybe the initial key issuer/provider (like gmail.com) signs username? then we hash it?
    - separate cluster/distributed datastore layer for managing the private key (using threshold signatures or something)
    - both create bottleneck... I guess this is fine since if you wanna email someone @domain.com anyways, you're assuming @domain.com is up.

 - Rate limiting: Any number of attackers should not be able to spam the network with proposals/queries. All queries/proposals must also compute a very simple proof-of-work with each request. The difficulty of the problem is also tuneable.
 
From a usability standpoint, we want to provide lightweight clients that have fast query times, allow users to add/change backup keys, and potentially allow for key recovery, all without compromising the above security requirements. A client can choose whether to run a full Stellar node; doing so rewards the client with faster request times (mbbe?). To query or alter a keyset, the client sends their request to a quorum slice. (How to do node discovery?)

All providers (eg email servers) should run full-nodes and audit each other~

### Bootstrapping existing PKI (root of trust)
We assume the existing CA infrastructure for mapping domains to PKs works properly, and only allow full cluster membership to machines with valid certs on an existing public keychain. The server is given the namespace associated with their cert's domain, and can start issuing keys for that namespace. For instance, Gmail servers can issue first keys for users @gmail.com, and Whatsapp servers can issue first keys for phone numbers @whatsapps server domain. Once this happens, if the end-user does not fully trust the issuing authority, they can immediately change their keyset by signing an update with the issued key.

### More fancy stuff
Sharding?

## The implementation
For the project, we've implemented PKI on top of a PBFT cluster and can send secure emails by querying the cluster :) PBFT was more tractable than implementing Stellar for the scope of this project. As a proof-of-concept, we first implemented the database on top of an existing Raft implementation, then wrote our own PBFT implementation and swapped it out. The abstractions we made were good, so swapping out raft for pbft wasn't bad, so switching out for an implementation of Stellar should b pretty e-z also.

## other work
 - stellar, lol
 - Certificate transparency: https://github.com/google/certificate-transparency
    - An auditable TLS certificate log.
 - Coniks: https://coniks.cs.princeton.edu/
    - An auditable and private name => public key database.
 - Key transparency: https://github.com/google/keytransparency
    - a Coniks DB hosted by Google.
 - Ethiks: http://www.jbonneau.com/doc/B16b-BITCOIN-ethiks.pdf
    - Using eth blockchain to audit Coniks.
 - Decentralized public key infrastructure (Rebooting Web of Trust): https://github.com/WebOfTrustInfo/rebooting-the-web-of-trust/blob/master/final-documents/dpki.pdf
    - Decentralized web-of-trust database. No privacy
