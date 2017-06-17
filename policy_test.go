package main

import (
	"log"
	"os"
	"testing"

	"github.com/ory/ladon"
)

var policyManager *PolicyManager

func init() {
	db := connect(os.Getenv("ORACLE_DSN"))
	policyManager = &PolicyManager{
		DB:    db,
		Table: randomTableName("pol"),
	}

	if _, err := policyManager.CreateSchemas(); err != nil {
		log.Fatalf("Could not create oauth2 schema table because %s", err)
	}
}

// This test is skipped because the method was deprecated
//
func TestFindPoliciesForSubject(t *testing.T) {
	ladon.TestHelperFindPoliciesForSubject("ora", policyManager)(t)
}

func TestCreateGetDelete(t *testing.T) {
	ladon.TestHelperCreateGetDelete(policyManager)(t)
}
