package lunixi

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

type canonicalFixture struct {
	PrivateSeed   string                                                       `json:"privateSeed"`
	PrivateKeyPEM string                                                       `json:"privateKeyPem"`
	Vectors       []struct{ Input, Body, Digest, Canonical, Signature string } `json:"vectors"`
}

func fixture(t *testing.T) canonicalFixture {
	t.Helper()
	data, e := os.ReadFile("testdata/canonical-vectors.json")
	if e != nil {
		t.Fatal(e)
	}
	var f canonicalFixture
	if e = json.Unmarshal(data, &f); e != nil {
		t.Fatal(e)
	}
	return f
}

func TestCanonicalMatchesIndependentNodeVectors(t *testing.T) {
	f := fixture(t)
	for _, v := range f.Vectors {
		body, e := CanonicalJSON(json.RawMessage(v.Input))
		if e != nil {
			t.Fatal(e)
		}
		if string(body) != v.Body {
			t.Fatalf("JSON differs: %s != %s", body, v.Body)
		}
		if Digest(body) != v.Digest {
			t.Fatal("digest differs from Node")
		}
		canonical := CanonicalRequest("post", "/api/v1/payments/intents?locale=tr", "2026-10-31T00:00:00Z", "0123456789abcdef0123456789abcdef", Digest(body))
		if canonical != v.Canonical {
			t.Fatal("canonical bytes differ from Node")
		}
		for _, material := range []string{f.PrivateSeed, f.PrivateKeyPEM} {
			key, e := parsePrivateKey(material)
			if e != nil {
				t.Fatal(e)
			}
			signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(canonical)))
			if signature != v.Signature {
				t.Fatal("Ed25519 signature differs from Node")
			}
		}
	}
}

func TestCanonicalRejectsDuplicateKeysAndUnsafeNumbers(t *testing.T) {
	for _, input := range []string{`{"amount":9007199254740992}`, `{"amount":1,"amount":2}`, `{"a":1} {"b":2}`, `NaN`} {
		if _, e := CanonicalJSON(json.RawMessage(input)); !errors.Is(e, ErrConfiguration) {
			t.Fatalf("accepted %s: %v", input, e)
		}
	}
	if _, e := CanonicalJSON(map[string]any{"amount": int64(MaxSafeInteger)}); e != nil {
		t.Fatal(e)
	}
}

func TestCanonicalNumericIndices(t *testing.T) {
	input := json.RawMessage(`{"4294967295":1,"9":2,"01":3,"0":4,"4294967294":5,"a":6}`)
	got, e := CanonicalJSON(input)
	if e != nil {
		t.Fatal(e)
	}
	if string(got) != `{"0":4,"9":2,"4294967294":5,"4294967295":1,"01":3,"a":6}` {
		t.Fatal(string(got))
	}
}

func TestPrivateKeyRejectsBadMaterial(t *testing.T) {
	for _, key := range []string{"", "not base64!", base64.StdEncoding.EncodeToString(make([]byte, 31)), "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----"} {
		if _, e := parsePrivateKey(key); e == nil {
			t.Fatal("accepted invalid key")
		}
	}
	seed, _ := base64.StdEncoding.DecodeString(fixture(t).PrivateSeed)
	key := ed25519.NewKeyFromSeed(seed)
	key[40] ^= 1
	if _, e := parsePrivateKey(base64.StdEncoding.EncodeToString(key)); e == nil {
		t.Fatal("accepted inconsistent public half")
	}
}

func TestExplicitFalseForce3D(t *testing.T) {
	disabled := false
	body, err := CanonicalJSON(IntentRequest{Force3D: &disabled})
	if err != nil || !bytes.Contains(body, []byte(`"force3D":false`)) {
		t.Fatalf("explicit false lost: %s %v", body, err)
	}
}
