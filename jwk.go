package main

import (
	"database/sql"
	"encoding/json"

	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/ory/hydra/jwk"
	"github.com/ory/hydra/pkg"
	"github.com/pkg/errors"
	"github.com/square/go-jose"
)

type JWKManager struct {
	DB     *sqlx.DB
	Cipher *jwk.AEAD
	Table  string
}

var jwkSchema = func(table string) string {
	return fmt.Sprintf(`CREATE TABLE %s (
	SID     NVARCHAR2(255) NOT NULL,
	KID 	NVARCHAR2(255) NOT NULL,
	VERSION INTEGER NOT NULL,
	KEYDATA VARCHAR2 (4000) NOT NULL,
	CONSTRAINT %s_pk_idx PRIMARY KEY (SID, KID)
)`, table, table)
}

type jwkSQLData struct {
	Set     string `db:"SID"`
	KID     string `db:"KID"`
	Version int    `db:"VERSION"`
	Key     string `db:"KEYDATA"`
}

func (m *JWKManager) GetTable() string {
	if m.Table == "" {
		return "hydjwk"
	}
	return m.Table
}

func (m *JWKManager) CreateSchemas() (int, error) {
	if _, err := m.DB.Exec(jwkSchema(m.GetTable())); err != nil {
		return 0, errors.Wrap(err, "Could not migrate jwk sql schema")
	}
	return 1, nil
}

func (m *JWKManager) AddKey(set string, key *jose.JsonWebKey) error {
	out, err := json.Marshal(key)
	if err != nil {
		return errors.WithStack(err)
	}

	encrypted, err := m.Cipher.Encrypt(out)
	if err != nil {
		return errors.WithStack(err)
	}

	query := fmt.Sprintf(`INSERT INTO %s (SID, KID, VERSION, KEYDATA) VALUES (:SID, :KID, :VERSION, :KEYDATA)`, m.GetTable())
	if _, err = m.DB.NamedExec(query, &jwkSQLData{
		Set:     set,
		KID:     key.KeyID,
		Version: 0,
		Key:     encrypted,
	}); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *JWKManager) AddKeySet(set string, keys *jose.JsonWebKeySet) error {
	tx, err := m.DB.Beginx()
	if err != nil {
		return errors.WithStack(err)
	}

	for _, key := range keys.Keys {
		out, err := json.Marshal(key)
		if err != nil {
			if re := tx.Rollback(); re != nil {
				return errors.Wrap(err, re.Error())
			}
			return errors.WithStack(err)
		}

		encrypted, err := m.Cipher.Encrypt(out)
		if err != nil {
			if re := tx.Rollback(); re != nil {
				return errors.Wrap(err, re.Error())
			}
			return errors.WithStack(err)
		}

		query := fmt.Sprintf(`INSERT INTO %s (SID, KID, VERSION, KEYDATA) VALUES (:SID, :KID, :VERSION, :KEYDATA)`, m.GetTable())
		if _, err = tx.NamedExec(query, &jwkSQLData{
			Set:     set,
			KID:     key.KeyID,
			Version: 0,
			Key:     encrypted,
		}); err != nil {
			if re := tx.Rollback(); re != nil {
				return errors.Wrap(err, re.Error())
			}
			return errors.WithStack(err)
		}
	}

	if err := tx.Commit(); err != nil {
		if re := tx.Rollback(); re != nil {
			return errors.Wrap(err, re.Error())
		}
		return errors.WithStack(err)
	}
	return nil
}

func (m *JWKManager) GetKey(set, KID string) (*jose.JsonWebKeySet, error) {
	var d jwkSQLData
	query := fmt.Sprintf("SELECT * FROM %s WHERE SID=? AND KID=?", m.GetTable())
	if err := m.DB.Get(&d, m.DB.Rebind(query), set, KID); err == sql.ErrNoRows {
		return nil, errors.Wrap(pkg.ErrNotFound, "")
	} else if err != nil {
		return nil, errors.WithStack(err)
	}

	key, err := m.Cipher.Decrypt(d.Key)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var c jose.JsonWebKey
	if err := json.Unmarshal(key, &c); err != nil {
		return nil, errors.WithStack(err)
	}

	return &jose.JsonWebKeySet{
		Keys: []jose.JsonWebKey{c},
	}, nil
}

func (m *JWKManager) GetKeySet(set string) (*jose.JsonWebKeySet, error) {
	var ds []jwkSQLData
	query := fmt.Sprintf("SELECT * FROM %s WHERE SID=?", m.GetTable())
	if err := m.DB.Select(&ds, m.DB.Rebind(query), set); err == sql.ErrNoRows {
		return nil, errors.Wrap(pkg.ErrNotFound, "")
	} else if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(ds) == 0 {
		return nil, errors.Wrap(pkg.ErrNotFound, "")
	}

	keys := &jose.JsonWebKeySet{Keys: []jose.JsonWebKey{}}
	for _, d := range ds {
		key, err := m.Cipher.Decrypt(d.Key)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		var c jose.JsonWebKey
		if err := json.Unmarshal(key, &c); err != nil {
			return nil, errors.WithStack(err)
		}
		keys.Keys = append(keys.Keys, c)
	}

	return keys, nil
}

func (m *JWKManager) DeleteKey(set, KID string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE SID=? AND KID=?`, m.GetTable())
	if _, err := m.DB.Exec(m.DB.Rebind(query), set, KID); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *JWKManager) DeleteKeySet(set string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE SID=?`, m.GetTable())
	if _, err := m.DB.Exec(m.DB.Rebind(query), set); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
