package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/ory/fosite"
	"github.com/ory/hydra/client"
	"github.com/ory/hydra/pkg"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

var clientSchema = func(testID string) string {
	s := fmt.Sprintf(`CREATE TABLE %s (
	ID      		varchar(255) NOT NULL PRIMARY KEY,
	CLIENT_NAME		VARCHAR2 (4000) NULL,
	CLIENT_SECRET  	VARCHAR2 (4000) NULL,
	REDIRECT_URIS  	VARCHAR2 (4000) NULL,
	GRANT_TYPES  	VARCHAR2 (4000) NULL,
	RESPONSE_TYPES  VARCHAR2 (4000) NULL,
	SCOPE  			VARCHAR2 (4000) NULL,
	OWNER  			VARCHAR2 (4000) NULL,
	POLICY_URI  	VARCHAR2 (4000) NULL,
	TOS_URI  		VARCHAR2 (4000) NULL,
	CLIENT_URI  	VARCHAR2 (4000) NULL,
	LOGO_URI  		VARCHAR2 (4000) NULL,
	CONTACTS  		VARCHAR2 (4000) NULL,
	IS_PUBLIC  		CHAR(1 BYTE) NOT NULL
)`, testID)
	return s
}

type ClientManager struct {
	Hasher fosite.Hasher
	DB     *sqlx.DB
	Table  string
}

type clientSqlData struct {
	ID                string `db:"ID"`
	Name              string `db:"CLIENT_NAME"`
	Secret            string `db:"CLIENT_SECRET"`
	RedirectURIs      string `db:"REDIRECT_URIS"`
	GrantTypes        string `db:"GRANT_TYPES"`
	ResponseTypes     string `db:"RESPONSE_TYPES"`
	Scope             string `db:"SCOPE"`
	Owner             string `db:"OWNER"`
	PolicyURI         string `db:"POLICY_URI"`
	TermsOfServiceURI string `db:"TOS_URI"`
	ClientURI         string `db:"CLIENT_URI"`
	LogoURI           string `db:"LOGO_URI"`
	Contacts          string `db:"CONTACTS"`
	Public            bool   `db:"IS_PUBLIC"`
}

var clientSqlParams = []string{
	"ID",
	"CLIENT_NAME",
	"CLIENT_SECRET",
	"REDIRECT_URIS",
	"GRANT_TYPES",
	"RESPONSE_TYPES",
	"SCOPE",
	"OWNER",
	"POLICY_URI",
	"TOS_URI",
	"CLIENT_URI",
	"LOGO_URI",
	"CONTACTS",
	"IS_PUBLIC",
}

func clientSqlDataFromClient(d *client.Client) *clientSqlData {
	return &clientSqlData{
		ID:                d.ID,
		Name:              d.Name,
		Secret:            d.Secret,
		RedirectURIs:      strings.Join(d.RedirectURIs, "|"),
		GrantTypes:        strings.Join(d.GrantTypes, "|"),
		ResponseTypes:     strings.Join(d.ResponseTypes, "|"),
		Scope:             d.Scope,
		Owner:             d.Owner,
		PolicyURI:         d.PolicyURI,
		TermsOfServiceURI: d.TermsOfServiceURI,
		ClientURI:         d.ClientURI,
		LogoURI:           d.LogoURI,
		Contacts:          strings.Join(d.Contacts, "|"),
		Public:            d.Public,
	}
}

func (d *clientSqlData) ToClient() *client.Client {
	return &client.Client{
		ID:                d.ID,
		Name:              d.Name,
		Secret:            d.Secret,
		RedirectURIs:      pkg.SplitNonEmpty(d.RedirectURIs, "|"),
		GrantTypes:        pkg.SplitNonEmpty(d.GrantTypes, "|"),
		ResponseTypes:     pkg.SplitNonEmpty(d.ResponseTypes, "|"),
		Scope:             d.Scope,
		Owner:             d.Owner,
		PolicyURI:         d.PolicyURI,
		TermsOfServiceURI: d.TermsOfServiceURI,
		ClientURI:         d.ClientURI,
		LogoURI:           d.LogoURI,
		Contacts:          pkg.SplitNonEmpty(d.Contacts, "|"),
		Public:            d.Public,
	}
}

func (m *ClientManager) CreateSchemas() (int, error) {
	if _, err := m.DB.Exec(clientSchema(m.GetTable())); err != nil {
		return 0, errors.Wrap(err, "Could not migrate client sql clientSchema")
	}
	return 1, nil
}

func (m *ClientManager) GetTable() string {
	if m.Table == "" {
		return "hydcl"
	}
	return m.Table
}

func (m *ClientManager) GetConcreteClient(ID string) (*client.Client, error) {
	var d clientSqlData
	if err := m.DB.Get(&d, m.DB.Rebind(fmt.Sprintf("SELECT * FROM %s WHERE ID=?", m.GetTable())), ID); err == sql.ErrNoRows {
		return nil, errors.Wrap(pkg.ErrNotFound, "")
	} else if err != nil {
		return nil, errors.WithStack(err)
	}

	return d.ToClient(), nil
}

func (m *ClientManager) GetClient(_ context.Context, ID string) (fosite.Client, error) {
	return m.GetConcreteClient(ID)
}

func (m *ClientManager) UpdateClient(c *client.Client) error {
	o, err := m.GetClient(context.Background(), c.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	if c.Secret == "" {
		c.Secret = string(o.GetHashedSecret())
	} else {
		h, err := m.Hasher.Hash([]byte(c.Secret))
		if err != nil {
			return errors.WithStack(err)
		}
		c.Secret = string(h)
	}

	s := clientSqlDataFromClient(c)
	var update []string
	for _, param := range clientSqlParams[1:] {
		update = append(update, fmt.Sprintf("%s=:%s", param, param))
	}

	if _, err := m.DB.NamedExec(fmt.Sprintf(`UPDATE %s SET %s WHERE ID=:ID`, m.GetTable(), strings.Join(update, ", ")), s); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *ClientManager) Authenticate(ID string, secret []byte) (*client.Client, error) {
	c, err := m.GetConcreteClient(ID)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := m.Hasher.Compare(c.GetHashedSecret(), secret); err != nil {
		return nil, errors.WithStack(err)
	}

	return c, nil
}

func (m *ClientManager) CreateClient(c *client.Client) error {
	if c.ID == "" {
		c.ID = uuid.New()
	}

	h, err := m.Hasher.Hash([]byte(c.Secret))
	if err != nil {
		return errors.WithStack(err)
	}
	c.Secret = string(h)

	data := clientSqlDataFromClient(c)
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		m.GetTable(),
		strings.Join(clientSqlParams, ", "),
		":"+strings.Join(clientSqlParams, ", :"),
	)
	if _, err := m.DB.NamedExec(query, data); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *ClientManager) DeleteClient(ID string) error {
	if _, err := m.DB.Exec(m.DB.Rebind(fmt.Sprintf(`DELETE FROM %s WHERE ID=?`, m.GetTable())), ID); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *ClientManager) GetClients() (clients map[string]client.Client, err error) {
	var d = []clientSqlData{}
	clients = make(map[string]client.Client)

	if err := m.DB.Select(&d, fmt.Sprintf("SELECT * FROM %s", m.GetTable())); err != nil {
		return nil, errors.WithStack(err)
	}

	for _, k := range d {
		clients[k.ID] = *k.ToClient()
	}
	return clients, nil
}
