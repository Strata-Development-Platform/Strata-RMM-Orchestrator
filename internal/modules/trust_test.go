package modules

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestParsePublisherTrustStoreJSON(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(publicKey)
	data := []byte(`[{"publisher":"example.publisher","key_id":"key-1","public_key":"` + encoded + `"}]`)
	store, err := ParsePublisherTrustStoreJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.LookupPublisherKey("example.publisher", "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if string(key.PublicKey) != string(publicKey) {
		t.Fatal("parsed public key does not match configured key")
	}
}

func TestParsePublisherTrustStoreJSONFailsClosed(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(publicKey)
	for name, data := range map[string]string{
		"empty":             `[]`,
		"unknown field":     `[{"publisher":"p","key_id":"k","public_key":"` + encoded + `","extra":true}]`,
		"bad key":           `[{"publisher":"p","key_id":"k","public_key":"not-base64"}]`,
		"duplicate":         `[{"publisher":"p","key_id":"k","public_key":"` + encoded + `"},{"publisher":"p","key_id":"k","public_key":"` + encoded + `"}]`,
		"trailing document": `[{"publisher":"p","key_id":"k","public_key":"` + encoded + `"}] {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePublisherTrustStoreJSON([]byte(data)); err == nil {
				t.Fatal("invalid trust configuration was accepted")
			}
		})
	}
}
