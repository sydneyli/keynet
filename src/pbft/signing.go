package pbft

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"golang.org/x/crypto/openpgp"
)

// ClientReply //

func (cr *ClientReply) generateDigest() ([sha256.Size]byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*cr); err != nil {
		var empty [sha256.Size]byte
		return empty, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

func (cr *ClientReply) SetDigest() {
	cr.digest = [sha256.Size]byte{}
	d, err := cr.generateDigest()
	if err != nil {
		plog.Fatal("Error setting ClientRequest digest")
	} else {
		cr.digest = d
	}
}

// PrePrepare //

func (pp *PrePrepare) Sign(node *openpgp.Entity) (*SignedPrePrepare, error) {
	var sig, buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*pp); err != nil {
		return nil, err
	}

	err := openpgp.DetachSign(&sig, node, &buf, nil)
	if err != nil {
		return nil, err
	}

	return &SignedPrePrepare{
		PrePrepareMessage: *pp,
		Signature:         sig.Bytes(),
	}, nil
}

func (pp *SignedPrePrepare) SignatureValid(peers openpgp.EntityList, peerMap map[EntityFingerprint]NodeId) (NodeId, error) {
	var buf, sig bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(pp.PrePrepareMessage); err != nil {
		return 0, err
	}

	if _, err := sig.Write(pp.Signature); err != nil {
		return 0, err
	}
	signer, err := openpgp.CheckDetachedSignature(peers, &buf, &sig)
	if err != nil {
		return 0, err
	}

	return peerMap[signer.PrimaryKey.Fingerprint], nil
}

// Enables RPC response messages without creating a new copy of the response
func (pp *PPResponse) GetSignature(node *openpgp.Entity) ([]byte, error) {
	var sig, buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*pp); err != nil {
		var emptyResult []byte
		return emptyResult, err
	}

	err := openpgp.DetachSign(&sig, node, &buf, nil)
	if err != nil {
		var emptyResult []byte
		return emptyResult, err
	}

	return sig.Bytes(), nil
}

func (pp *PPResponse) Sign(node *openpgp.Entity) (*SignedPPResponse, error) {
	sig, err := pp.GetSignature(node)
	if err != nil {
		return nil, err
	}

	return &SignedPPResponse{
		Response:  *pp,
		Signature: sig,
	}, nil
}

func (pp *SignedPPResponse) SignatureValid(peers openpgp.EntityList, peerMap map[EntityFingerprint]NodeId) (NodeId, error) {
	var buf, sig bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(pp.Response); err != nil {
		return 0, err
	}

	if _, err := sig.Write(pp.Signature); err != nil {
		return 0, err
	}
	signer, err := openpgp.CheckDetachedSignature(peers, &buf, &sig)
	if err != nil {
		return 0, err
	}

	return peerMap[signer.PrimaryKey.Fingerprint], nil
}

// Prepare //

func (p *Prepare) Sign(node *openpgp.Entity) (*SignedPrepare, error) {
	var sig, buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*p); err != nil {
		return nil, err
	}

	err := openpgp.DetachSign(&sig, node, &buf, nil)
	if err != nil {
		return nil, err
	}

	return &SignedPrepare{
		PrepareMessage: *p,
		Signature:      sig.Bytes(),
	}, nil
}

func (p *SignedPrepare) SignatureValid(peers openpgp.EntityList, peerMap map[EntityFingerprint]NodeId) (NodeId, error) {
	var buf, sig bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(p.PrepareMessage); err != nil {
		return 0, err
	}

	if _, err := sig.Write(p.Signature); err != nil {
		return 0, err
	}
	signer, err := openpgp.CheckDetachedSignature(peers, &buf, &sig)
	if err != nil {
		return 0, err
	}

	return peerMap[signer.PrimaryKey.Fingerprint], nil
}

// Commit //

func (c *Commit) Sign(node *openpgp.Entity) (*SignedCommit, error) {
	var sig, buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*c); err != nil {
		return nil, err
	}

	err := openpgp.DetachSign(&sig, node, &buf, nil)
	if err != nil {
		return nil, err
	}

	return &SignedCommit{
		CommitMessage: *c,
		Signature:     sig.Bytes(),
	}, nil
}

func (c *SignedCommit) SignatureValid(peers openpgp.EntityList, peerMap map[EntityFingerprint]NodeId) (NodeId, error) {
	var buf, sig bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(c.CommitMessage); err != nil {
		return 0, err
	}

	if _, err := sig.Write(c.Signature); err != nil {
		return 0, err
	}
	signer, err := openpgp.CheckDetachedSignature(peers, &buf, &sig)
	if err != nil {
		return 0, err
	}

	return peerMap[signer.PrimaryKey.Fingerprint], nil
}

// Checkpoint //

func (c *Checkpoint) Sign(node *openpgp.Entity) (*SignedCheckpoint, error) {
	var sig, buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*c); err != nil {
		return nil, err
	}

	err := openpgp.DetachSign(&sig, node, &buf, nil)
	if err != nil {
		return nil, err
	}

	return &SignedCheckpoint{
		CheckpointMessage: *c,
		Signature:         sig.Bytes(),
	}, nil
}

func (c *SignedCheckpoint) SignatureValid(peers openpgp.EntityList, peerMap map[EntityFingerprint]NodeId) (NodeId, error) {
	var buf, sig bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(c.CheckpointMessage); err != nil {
		return 0, err
	}

	if _, err := sig.Write(c.Signature); err != nil {
		return 0, err
	}
	signer, err := openpgp.CheckDetachedSignature(peers, &buf, &sig)
	if err != nil {
		return 0, err
	}

	return peerMap[signer.PrimaryKey.Fingerprint], nil
}

// ViewChange //

func (vc *ViewChange) generateDigest() ([sha256.Size]byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*vc); err != nil {
		var empty [sha256.Size]byte
		return empty, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

func (vc *ViewChange) SetDigest() {
	vc.Digest = [sha256.Size]byte{}
	d, err := vc.generateDigest()
	if err != nil {
		plog.Fatal("Error setting ViewChange digest")
	} else {
		vc.Digest = d
	}
}

func (nv *NewView) generateDigest() ([sha256.Size]byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*nv); err != nil {
		var empty [sha256.Size]byte
		return empty, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

func (nv *NewView) SetDigest() {
	nv.Digest = [sha256.Size]byte{}
	d, err := nv.generateDigest()
	if err != nil {
		plog.Fatal("Error setting NewView digest")
	} else {
		nv.Digest = d
	}
}

func (cr *ClientReply) DigestValid() bool {
	currentDigest := cr.digest
	cr.digest = [sha256.Size]byte{}
	d, err := cr.generateDigest()
	if err != nil {
		plog.Fatal("Error calculating ClientReply digest for validity")
		return false
	} else {
		cr.digest = currentDigest
		return d == currentDigest
	}
}

func (vc *ViewChange) DigestValid() bool {
	currentDigest := vc.Digest
	vc.Digest = [sha256.Size]byte{}
	d, err := vc.generateDigest()
	if err != nil {
		plog.Fatal("Error calculating ViewChange digest for validity")
		return false
	} else {
		vc.Digest = currentDigest
		return d == currentDigest
	}
}

func (nv *NewView) DigestValid() bool {
	currentDigest := nv.Digest
	nv.Digest = [sha256.Size]byte{}
	d, err := nv.generateDigest()
	if err != nil {
		plog.Fatal("Error calculating NewView digest for validity")
		return false
	} else {
		nv.Digest = currentDigest
		return d == currentDigest
	}
}
