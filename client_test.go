package main

import (
	"testing"

	"log"
	"os"

	_ "github.com/lib/pq"
	"github.com/ory/fosite"
	"github.com/ory/hydra/client"
)

var clientManager *ClientManager

func init() {
	db := connect(os.Getenv("ORACLE_DSN"))
	clientManager = &ClientManager{
		DB:     db,
		Hasher: &fosite.BCrypt{WorkFactor: 4},
		Table:  randomTableName("oauth2"),
	}

	if _, err := clientManager.CreateSchemas(); err != nil {
		log.Fatalf("Could not create client schema table %s because %s", clientManager.GetTable(), err)
	}
}

func TestClientAutoGenerateKey(t *testing.T) {
	client.TestHelperClientAutoGenerateKey("ora", clientManager)(t)
}

func TestCreateGetDeleteClient(t *testing.T) {
	client.TestHelperCreateGetDeleteClient("ora", clientManager)(t)
}

func TestAuthenticateClient(t *testing.T) {
	client.TestHelperClientAuthenticate("ora", clientManager)(t)
}
