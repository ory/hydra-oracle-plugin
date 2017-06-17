package main

import "C"

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/ory/fosite"
	"github.com/ory/hydra/client"
	"github.com/ory/hydra/jwk"
	"github.com/ory/hydra/pkg"
	"github.com/ory/hydra/warden/group"
	"github.com/ory/ladon"
	"github.com/pkg/errors"
	_ "gopkg.in/rana/ora.v4"
)

func main() {
	Execute()
}

func Connect(u string) (*sqlx.DB, error) {
	host, database := GetDatabase(u)
	db, err := sqlx.Open("ora", host)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := db.Ping(); err != nil {
		return nil, errors.WithStack(err)
	}

	if _, err := db.Exec(fmt.Sprintf("ALTER SESSION SET CURRENT_SCHEMA = %s", database)); err != nil {
		return nil, errors.WithStack(err)
	}

	return db, nil
}

func NewClientManager(db *sqlx.DB, hasher fosite.Hasher) client.Manager {
	return &ClientManager{
		DB:     db,
		Hasher: hasher,
		Table:  "hyd_clt",
	}
}

func NewGroupManager(db *sqlx.DB) group.Manager {
	return &GroupManager{
		DB:    db,
		Table: "hyd_grp",
	}
}

func NewJWKManager(db *sqlx.DB, cipher *jwk.AEAD) jwk.Manager {
	return &JWKManager{
		DB:     db,
		Cipher: cipher,
		Table:  "hyd_jwk",
	}
}

func NewOAuth2Manager(db *sqlx.DB, cm client.Manager, logger logrus.FieldLogger) pkg.FositeStorer {
	return &FositeStore{
		Manager: cm,
		DB:      db,
		L:       logger,
		Table:   "hyd_oa2",
	}
}

func NewPolicyManager(db *sqlx.DB) ladon.Manager {
	return &PolicyManager{
		DB:    db,
		Table: "hyd_pol",
	}
}

func CreateSchemas(db *sqlx.DB) error {
	if _, err := (&ClientManager{
		DB: db,
		Table:  "hyd_clt",
	}).CreateSchemas(); err != nil {
		return errors.WithStack(err)
	}
	if _, err := (&GroupManager{
		DB: db,
		Table: "hyd_grp",
	}).CreateSchemas(); err != nil {
		return errors.WithStack(err)
	}
	if _, err := (&JWKManager{
		DB: db,
		Table:  "hyd_jwk",
	}).CreateSchemas(); err != nil {
		return errors.WithStack(err)
	}
	if _, err := (&FositeStore{
		DB: db,
		Table:   "hyd_oa2",
	}).CreateSchemas(); err != nil {
		return errors.WithStack(err)
	}
	if _, err := (&PolicyManager{
		DB: db,
		Table: "hyd_pol",
	}).CreateSchemas(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}
