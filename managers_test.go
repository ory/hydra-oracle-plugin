package main

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/ory/hydra/rand/sequence"
	_ "gopkg.in/rana/ora.v4"
)

func connect(url string) *sqlx.DB {
	host, database := GetDatabase(url)
	db, err := sqlx.Open("ora", host)
	if err != nil {
		log.Fatalf("Could not connect to SQL instance: %s", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("Could not ping SQL instance: %s", err)
	}

	if _, err := db.Exec(fmt.Sprintf("ALTER SESSION SET CURRENT_SCHEMA = %s", database)); err != nil {
		log.Fatalf("Could not select database %s: %s", database, err)
	}

	return db
}

func randomTableName(prefix string) string {
	tbl, _ := sequence.RuneSequence(10, []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	return prefix + "_" + string(tbl)
}
