package main

import (
	"log"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/ory/fosite"
	"github.com/ory/hydra/client"
	"github.com/ory/hydra/oauth2"
)

var oauth2Manager *FositeStore

func init() {
	db := connect(os.Getenv("ORACLE_DSN"))
	cm := &client.MemoryManager{
		Clients: map[string]client.Client{"foobar": {ID: "foobar"}},
		Hasher:  &fosite.BCrypt{},
	}
	oauth2Manager = &FositeStore{
		Manager: cm,
		DB:      db,
		L:       logrus.StandardLogger(),
		Table:   randomTableName("oauth2"),
	}

	if _, err := oauth2Manager.CreateSchemas(); err != nil {
		log.Fatalf("Could not create oauth2 schema table because %s", err)
	}
}

func TestCreateGetDeleteAuthorizeCodes(t *testing.T) {
	oauth2.TestHelperCreateGetDeleteAuthorizeCodes(oauth2Manager)(t)
}

func TestCreateGetDeleteAccessTokenSession(t *testing.T) {
	oauth2.TestHelperCreateGetDeleteAccessTokenSession(oauth2Manager)(t)
}

func TestCreateGetDeleteOpenIDConnectSession(t *testing.T) {
	oauth2.TestHelperCreateGetDeleteOpenIDConnectSession(oauth2Manager)(t)
}

func TestCreateGetDeleteRefreshTokenSession(t *testing.T) {
	oauth2.TestHelperCreateGetDeleteRefreshTokenSession(oauth2Manager)(t)
}

func TestRevokeRefreshToken(t *testing.T) {
	oauth2.TestHelperRevokeRefreshToken(oauth2Manager)(t)
}
