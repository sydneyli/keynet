package pbft

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
)

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

func (pp *PrePrepare) generateDigest() ([sha256.Size]byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*pp); err != nil {
		var empty [sha256.Size]byte
		return empty, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

func (pp *PrePrepare) SetDigest() {
	pp.Digest = [sha256.Size]byte{}
	d, err := pp.generateDigest()
	if err != nil {
		plog.Fatal("Error setting PrePrepare digest")
	} else {
		pp.Digest = d
	}
}

func (p *Prepare) generateDigest() ([sha256.Size]byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*p); err != nil {
		var empty [sha256.Size]byte
		return empty, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

func (p *Prepare) SetDigest() {
	p.Digest = [sha256.Size]byte{}
	d, err := p.generateDigest()
	if err != nil {
		plog.Fatal("Error setting Prepare digest")
	} else {
		p.Digest = d
	}
}

func (c *Commit) generateDigest() ([sha256.Size]byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(*c); err != nil {
		var empty [sha256.Size]byte
		return empty, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

func (c *Commit) SetDigest() {
	c.Digest = [sha256.Size]byte{}
	d, err := c.generateDigest()
	if err != nil {
		plog.Fatal("Error setting Commit digest")
	} else {
		c.Digest = d
	}
}

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

func (pp *PrePrepare) DigestValid() bool {
	currentDigest := pp.Digest
	pp.Digest = [sha256.Size]byte{}

	d, err := pp.generateDigest()
	if err != nil {
		plog.Fatal("Error calculating PrePrepare digest for validity")
		return false
	} else {
		pp.Digest = currentDigest
		return d == currentDigest
	}
}

func (p *Prepare) DigestValid() bool {
	currentDigest := p.Digest
	p.Digest = [sha256.Size]byte{}
	d, err := p.generateDigest()
	if err != nil {
		plog.Fatal("Error calculating Prepare digest for validity")
		return false
	} else {
		p.Digest = currentDigest
		return d == currentDigest
	}
}

func (c *Commit) DigestValid() bool {
	currentDigest := c.Digest
	c.Digest = [sha256.Size]byte{}
	d, err := c.generateDigest()
	if err != nil {
		plog.Fatal("Error calculating Commit digest for validity")
		return false
	} else {
		c.Digest = currentDigest
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
