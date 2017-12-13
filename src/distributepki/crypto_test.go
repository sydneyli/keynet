package main

import (
	"strings"
	"testing"

	"golang.org/x/crypto/openpgp"
)

/* Make sure signature generation/validation works */
func TestSignature(t *testing.T) {
	data := "\"abcd\""
	key :=
		`-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: Mailvelope v2.0.0 build: 2017-12-11T17:18:42
Comment: https://www.mailvelope.com

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
KWnDCfxiiy/amuhQuY2Am1D7oyhs
=Ea8O
-----END PGP PUBLIC KEY BLOCK-----`
	signature :=
		`-----BEGIN PGP SIGNATURE-----
Version: Mailvelope v2.0.0 build: 2017-12-11T17:18:42
Comment: https://www.mailvelope.com

wsFcBAEBCAAQBQJaLr4NCRByZvS92PsxlwAApvkP/2kMWr/LKqICul+COD++
aY6pmERPS3JiBKMTUIYxMiAd6S+4FuC5aq+nDyetjuMJ0WKnNoSJVf9TZrEJ
U5LzqLxB3XQ7mrtg0cH5ZbLvcNKzhJCH9yx9pINYQw+yhu6xsZpUGNwXryMM
PE+Lkaw7PDFkpHNWv7vnHsCusq3H6VqhBBtcXxFVRR+7PQxve+GoK0Iiqywg
GFvwTBD9l8homCwVFPqWl0vCnbvRlQr7sflIbzRocdYsacaX8n8RKdF8tSFg
BeCrWnP48U9o2VU9Iy23hyG2JLlGgwXVV/nJKNad1GA5P9HuUeeb8382r8rV
Z4L65j+na+iif8gajJiT/+80dxOBX0K6b4ztUQZB8xewZHvBQlkl6gP3/0jc
I4nI304t0fkGEjWYqaD3+14mZCqNRemvmfkx/3WfzEGMHfNhj98KOBLs0mnP
G5y9OTY3maLG/lj9x4Lkcu1f5gdY2zspYSYIfMgVKp30dDMep++fNgUEZyvf
XUjTeGjcFQuqSTGxYPNdCom6rh9LDGq4M+MFGu5AFiZzAp1/UF8jAQQNwKuX
F9T5O/mzPT+MPGaiazGodIT/7cuJ4iuLj7481pXxplxDQ5gPOSb5LBvwNQkf
PFhsu4JH8mR21X/7WPebtl7TjZ4kASDBiad1dhlhfOPvQgt/nn25OgQtRNXv
ST6M
=hkIa
-----END PGP SIGNATURE-----`
	keyring, readErr := openpgp.ReadArmoredKeyRing(strings.NewReader(key))
	if readErr != nil {
		t.Error("Couldn't read previous key!")
		t.Fatal("failed")
	}
	for _, key := range keyring {
		t.Logf("%+v", key.PrimaryKey)
	}
	_, err := openpgp.CheckArmoredDetachedSignature(keyring, strings.NewReader(data), strings.NewReader(signature))
	if err != nil {
		t.Error(err)
		t.Fatal("failed")
	}
}
