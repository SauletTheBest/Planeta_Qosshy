package util

import "testing"

func TestNewToken(t *testing.T) {
	raw1, hash1, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	raw2, _, _ := NewToken()
	if raw1 == raw2 {
		t.Fatal("two tokens are equal")
	}
	if len(raw1) != 43 { // 32 bytes in base64 without padding
		t.Fatalf("token length %d", len(raw1))
	}
	if hash1 != HashToken(raw1) || hash1 == raw1 || len(hash1) != 64 {
		t.Fatalf("hash %q of %q", hash1, raw1)
	}
}

// The value Postgres computes in migration 000008:
// SELECT encode(sha256(convert_to('abc', 'UTF8')), 'hex')
func TestHashTokenMatchesPostgres(t *testing.T) {
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := HashToken("abc"); got != want {
		t.Fatalf("got %s", got)
	}
}
