package main

import (
	"log"
	"os"
	"testing"

	"github.com/ory/hydra/jwk"
)

var testGenerator = &jwk.RS256Generator{}
var jwkManager *JWKManager

func init() {
	encryptionKey, _ := jwk.RandomBytes(32)
	db := connect(os.Getenv("ORACLE_DSN"))
	jwkManager = &JWKManager{
		DB:     db,
		Cipher: &jwk.AEAD{Key: encryptionKey},
		Table:  randomTableName("jwk"),
	}

	if _, err := jwkManager.CreateSchemas(); err != nil {
		log.Fatalf("Could not create jwk schema table %s because %s", jwkManager.GetTable(), err)
	}
}

func TestManagerKey(t *testing.T) {
	ks, _ := testGenerator.Generate("")
	jwk.TestHelperManagerKey(jwkManager, ks)(t)
}

func TestManagerKeySet(t *testing.T) {
	ks, _ := testGenerator.Generate("")
	jwk.TestHelperManagerKeySet(jwkManager, ks)(t)
}
