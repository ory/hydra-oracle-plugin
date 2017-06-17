package main

import (
	"log"
	"os"
	"testing"

	"github.com/ory/hydra/warden/group"
)

var groupManager *GroupManager

func init() {
	db := connect(os.Getenv("ORACLE_DSN"))
	groupManager = &GroupManager{
		DB:    db,
		Table: randomTableName("group"),
	}

	if _, err := groupManager.CreateSchemas(); err != nil {
		log.Fatalf("Could not create group schema table because %s", err)
	}
}

func TestManagers(t *testing.T) {
	group.TestHelperManagers(groupManager)(t)
}
