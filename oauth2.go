package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/ory/fosite"
	"github.com/ory/hydra/client"
	"github.com/pkg/errors"
)

type FositeStore struct {
	client.Manager
	DB    *sqlx.DB
	L     logrus.FieldLogger
	Table string
}

func (m *FositeStore) GetTable() string {
	if m.Table == "" {
		return "hydoa2"
	}
	return m.Table
}

func fositeSqlTemplate(kind, table string) string {
	return fmt.Sprintf(`CREATE TABLE %s_%s (
	SIGNATURE      	varchar(255) NOT NULL PRIMARY KEY,
	REQUEST_ID  	varchar(255) NOT NULL,
	REQUESTED_AT  	TIMESTAMP NOT NULL,
	CLIENT_ID  		VARCHAR2 (4000) NULL,
	SCOPE  			VARCHAR2 (4000) NULL,
	GRANTED_SCOPE 	VARCHAR2 (4000) NULL,
	FORM_DATA  		VARCHAR2 (4000) NULL,
	SESSION_DATA  	VARCHAR2 (4000) NULL
)`, table, kind)
}

const (
	sqlTableOpenID  = "o"
	sqlTableAccess  = "a"
	sqlTableRefresh = "r"
	sqlTableCode    = "c"
)

var sqlParams = []string{
	"SIGNATURE",
	"REQUEST_ID",
	"REQUESTED_AT",
	"CLIENT_ID",
	"SCOPE",
	"GRANTED_SCOPE",
	"FORM_DATA",
	"SESSION_DATA",
}

type sqlData struct {
	Signature     string    `db:"SIGNATURE"`
	Request       string    `db:"REQUEST_ID"`
	RequestedAt   time.Time `db:"REQUESTED_AT"`
	Client        string    `db:"CLIENT_ID"`
	Scopes        string    `db:"SCOPE"`
	GrantedScopes string    `db:"GRANTED_SCOPE"`
	Form          string    `db:"FORM_DATA"`
	Session       string    `db:"SESSION_DATA"`
}

func fositeSqlSchemaFromRequest(SIGNATURE string, r fosite.Requester, logger logrus.FieldLogger) (*sqlData, error) {
	if r.GetSession() == nil {
		logger.Debugf("Got an empty session in fositeSqlSchemaFromRequest")
	}

	session, err := json.Marshal(r.GetSession())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &sqlData{
		Request:       r.GetID(),
		Signature:     SIGNATURE,
		RequestedAt:   r.GetRequestedAt(),
		Client:        r.GetClient().GetID(),
		Scopes:        strings.Join([]string(r.GetRequestedScopes()), "|"),
		GrantedScopes: strings.Join([]string(r.GetGrantedScopes()), "|"),
		Form:          r.GetRequestForm().Encode(),
		Session:       string(session),
	}, nil
}

func (s *sqlData) toRequest(session fosite.Session, cm client.Manager, logger logrus.FieldLogger) (*fosite.Request, error) {
	if session != nil {
		if err := json.Unmarshal([]byte(s.Session), session); err != nil {
			return nil, errors.Wrapf(err, "Could not unmarshal session data: %s", s.Session)
		}
	} else {
		logger.Debugf("Got an empty session in toRequest")
	}

	c, err := cm.GetClient(context.Background(), s.Client)
	if err != nil {
		return nil, err
	}

	val, err := url.ParseQuery(s.Form)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	r := &fosite.Request{
		ID:            s.Request,
		RequestedAt:   s.RequestedAt,
		Client:        c,
		Scopes:        fosite.Arguments(strings.Split(s.Scopes, "|")),
		GrantedScopes: fosite.Arguments(strings.Split(s.GrantedScopes, "|")),
		Form:          val,
		Session:       session,
	}

	return r, nil
}

func (s *FositeStore) createSession(SIGNATURE string, requester fosite.Requester, table string) error {
	data, err := fositeSqlSchemaFromRequest(SIGNATURE, requester, s.L)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(
		"INSERT INTO %s_%s (%s) VALUES (%s)",
		s.GetTable(),
		table,
		strings.Join(sqlParams, ", "),
		":"+strings.Join(sqlParams, ", :"),
	)
	if _, err := s.DB.NamedExec(query, data); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (s *FositeStore) findSessionBySignature(SIGNATURE string, session fosite.Session, table string) (fosite.Requester, error) {
	var d sqlData
	if err := s.DB.Get(&d, s.DB.Rebind(fmt.Sprintf("SELECT * FROM %s_%s WHERE SIGNATURE=?", s.GetTable(), table)), SIGNATURE); err == sql.ErrNoRows {
		return nil, errors.Wrap(fosite.ErrNotFound, "")
	} else if err != nil {
		return nil, errors.WithStack(err)
	}

	return d.toRequest(session, s.Manager, s.L)
}

func (s *FositeStore) deleteSession(SIGNATURE string, table string) error {
	if _, err := s.DB.Exec(s.DB.Rebind(fmt.Sprintf("DELETE FROM %s_%s WHERE SIGNATURE=?", s.GetTable(), table)), SIGNATURE); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (s *FositeStore) CreateSchemas() (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, errors.WithStack(err)
	}

	for _, schema := range []string{
		fositeSqlTemplate(sqlTableAccess, s.GetTable()),
		fositeSqlTemplate(sqlTableRefresh, s.GetTable()),
		fositeSqlTemplate(sqlTableCode, s.GetTable()),
		fositeSqlTemplate(sqlTableOpenID, s.GetTable()),
	} {
		if _, err := tx.Exec(schema); err != nil {
			if err := tx.Rollback(); err != nil {
				return 0, errors.WithStack(err)
			}
			return 0, errors.Wrapf(err, "Could not migrate oauth2 sql schema: %s", schema)
		}
	}

	if err := tx.Commit(); err != nil {
		if err := tx.Rollback(); err != nil {
			return 0, errors.WithStack(err)
		}
		return 0, errors.WithStack(err)
	}

	return 1, nil
}

func (s *FositeStore) CreateOpenIDConnectSession(_ context.Context, SIGNATURE string, requester fosite.Requester) error {
	return s.createSession(SIGNATURE, requester, sqlTableOpenID)
}

func (s *FositeStore) GetOpenIDConnectSession(_ context.Context, SIGNATURE string, requester fosite.Requester) (fosite.Requester, error) {
	return s.findSessionBySignature(SIGNATURE, requester.GetSession(), sqlTableOpenID)
}

func (s *FositeStore) DeleteOpenIDConnectSession(_ context.Context, SIGNATURE string) error {
	return s.deleteSession(SIGNATURE, sqlTableOpenID)
}

func (s *FositeStore) CreateAuthorizeCodeSession(_ context.Context, SIGNATURE string, requester fosite.Requester) error {
	return s.createSession(SIGNATURE, requester, sqlTableCode)
}

func (s *FositeStore) GetAuthorizeCodeSession(_ context.Context, SIGNATURE string, session fosite.Session) (fosite.Requester, error) {
	return s.findSessionBySignature(SIGNATURE, session, sqlTableCode)
}

func (s *FositeStore) DeleteAuthorizeCodeSession(_ context.Context, SIGNATURE string) error {
	return s.deleteSession(SIGNATURE, sqlTableCode)
}

func (s *FositeStore) CreateAccessTokenSession(_ context.Context, SIGNATURE string, requester fosite.Requester) error {
	return s.createSession(SIGNATURE, requester, sqlTableAccess)
}

func (s *FositeStore) GetAccessTokenSession(_ context.Context, SIGNATURE string, session fosite.Session) (fosite.Requester, error) {
	return s.findSessionBySignature(SIGNATURE, session, sqlTableAccess)
}

func (s *FositeStore) DeleteAccessTokenSession(_ context.Context, SIGNATURE string) error {
	return s.deleteSession(SIGNATURE, sqlTableAccess)
}

func (s *FositeStore) CreateRefreshTokenSession(_ context.Context, SIGNATURE string, requester fosite.Requester) error {
	return s.createSession(SIGNATURE, requester, sqlTableRefresh)
}

func (s *FositeStore) GetRefreshTokenSession(_ context.Context, SIGNATURE string, session fosite.Session) (fosite.Requester, error) {
	return s.findSessionBySignature(SIGNATURE, session, sqlTableRefresh)
}

func (s *FositeStore) DeleteRefreshTokenSession(_ context.Context, SIGNATURE string) error {
	return s.deleteSession(SIGNATURE, sqlTableRefresh)
}

func (s *FositeStore) CreateImplicitAccessTokenSession(ctx context.Context, SIGNATURE string, requester fosite.Requester) error {
	return s.CreateAccessTokenSession(ctx, SIGNATURE, requester)
}

func (s *FositeStore) PersistAuthorizeCodeGrantSession(ctx context.Context, authorizeCode, accessSignature, refreshSignature string, request fosite.Requester) error {
	if err := s.DeleteAuthorizeCodeSession(ctx, authorizeCode); err != nil {
		return err
	} else if err := s.CreateAccessTokenSession(ctx, accessSignature, request); err != nil {
		return err
	}

	if refreshSignature == "" {
		return nil
	}

	if err := s.CreateRefreshTokenSession(ctx, refreshSignature, request); err != nil {
		return err
	}

	return nil
}

func (s *FositeStore) PersistRefreshTokenGrantSession(ctx context.Context, originalRefreshSignature, accessSignature, refreshSignature string, request fosite.Requester) error {
	if err := s.DeleteRefreshTokenSession(ctx, originalRefreshSignature); err != nil {
		return err
	} else if err := s.CreateAccessTokenSession(ctx, accessSignature, request); err != nil {
		return err
	} else if err := s.CreateRefreshTokenSession(ctx, refreshSignature, request); err != nil {
		return err
	}

	return nil
}

func (s *FositeStore) RevokeRefreshToken(ctx context.Context, id string) error {
	return s.revokeSession(id, sqlTableRefresh)
}

func (s *FositeStore) RevokeAccessToken(ctx context.Context, id string) error {
	return s.revokeSession(id, sqlTableAccess)
}

func (s *FositeStore) revokeSession(id string, table string) error {
	if _, err := s.DB.Exec(s.DB.Rebind(fmt.Sprintf("DELETE FROM %s_%s WHERE REQUEST_ID=?", s.GetTable(), table)), id); err == sql.ErrNoRows {
		return errors.Wrap(fosite.ErrNotFound, "")
	} else if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
