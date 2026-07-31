package platform

import (
	"encoding/hex"
	"testing"
)

func TestDecodeAgentPublicKey(t *testing.T) {
	valid := make([]byte, 65)
	valid[0] = 4
	decoded, err := decodeAgentPublicKey(hex.EncodeToString(valid))
	if err != nil {
		t.Fatalf("valid uncompressed key rejected: %v", err)
	}
	if len(decoded) != 65 {
		t.Fatalf("decoded key length = %d", len(decoded))
	}

	for _, malformed := range []string{"", "not-hex", "04", hex.EncodeToString(make([]byte, 65))} {
		if _, err := decodeAgentPublicKey(malformed); err == nil {
			t.Fatalf("malformed key %q accepted", malformed)
		}
	}
}
