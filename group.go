package main

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/ory/hydra/warden/group"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

var groupSchema = func(table string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE %s (
	id      	varchar(255) NOT NULL PRIMARY KEY
)`, table),
		fmt.Sprintf(`CREATE TABLE %s_m (
	member		varchar(255) NOT NULL,
	group_id	varchar(255) NOT NULL,
	FOREIGN KEY (group_id) REFERENCES %s(id) ON DELETE CASCADE,
	PRIMARY KEY (member, group_id)
)`, table, table),
	}
}

type GroupManager struct {
	DB    *sqlx.DB
	Table string
}

func (m *GroupManager) GetTable() string {
	if m.Table == "" {
		return "hydgr"
	}
	return m.Table
}

func (m *GroupManager) CreateSchemas() (int, error) {
	tx, err := m.DB.Begin()
	if err != nil {
		return 0, errors.WithStack(err)
	}

	for _, schema := range groupSchema(m.GetTable()) {
		if _, err := tx.Exec(schema); err != nil {
			if err := tx.Rollback(); err != nil {
				return 0, errors.WithStack(err)
			}
			return 0, errors.Wrapf(err, "Could not migrate group sql schema: %s", schema)
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

func (m *GroupManager) CreateGroup(g *group.Group) error {
	if g.ID == "" {
		g.ID = uuid.New()
	}

	query := fmt.Sprintf("INSERT INTO %s (id) VALUES (?)", m.GetTable())
	if _, err := m.DB.Exec(m.DB.Rebind(query), g.ID); err != nil {
		return errors.WithStack(err)
	}

	return m.AddGroupMembers(g.ID, g.Members)
}

func (m *GroupManager) GetGroup(id string) (*group.Group, error) {
	var found string
	query := fmt.Sprintf("SELECT id from %s WHERE id = ?", m.GetTable())
	if err := m.DB.Get(&found, m.DB.Rebind(query), id); err != nil {
		return nil, errors.WithStack(err)
	}

	var q []string
	query = fmt.Sprintf("SELECT member from %s_m WHERE group_id = ?", m.GetTable())
	if err := m.DB.Select(&q, m.DB.Rebind(query), found); err != nil {
		return nil, errors.WithStack(err)
	}

	return &group.Group{
		ID:      found,
		Members: q,
	}, nil
}

func (m *GroupManager) DeleteGroup(id string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id=?", m.GetTable())
	if _, err := m.DB.Exec(m.DB.Rebind(query), id); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *GroupManager) AddGroupMembers(group string, subjects []string) error {
	tx, err := m.DB.Beginx()
	if err != nil {
		return errors.Wrap(err, "Could not begin transaction")
	}

	query := fmt.Sprintf("INSERT INTO %s_m (group_id, member) VALUES (?, ?)", m.GetTable())
	for _, subject := range subjects {
		if _, err := tx.Exec(m.DB.Rebind(query), group, subject); err != nil {
			if err := tx.Rollback(); err != nil {
				return errors.WithStack(err)
			}
			return errors.WithStack(err)
		}
	}

	if err := tx.Commit(); err != nil {
		if err := tx.Rollback(); err != nil {
			return errors.WithStack(err)
		}
		return errors.Wrap(err, "Could not commit transaction")
	}
	return nil
}

func (m *GroupManager) RemoveGroupMembers(group string, subjects []string) error {
	tx, err := m.DB.Beginx()
	if err != nil {
		return errors.Wrap(err, "Could not begin transaction")
	}

	query := fmt.Sprintf("DELETE FROM %s_m WHERE member=? AND group_id=?", m.GetTable())
	for _, subject := range subjects {
		if _, err := m.DB.Exec(m.DB.Rebind(query), subject, group); err != nil {
			if err := tx.Rollback(); err != nil {
				return errors.WithStack(err)
			}
			return errors.WithStack(err)
		}
	}

	if err := tx.Commit(); err != nil {
		if err := tx.Rollback(); err != nil {
			return errors.WithStack(err)
		}
		return errors.Wrap(err, "Could not commit transaction")
	}
	return nil
}

func (m *GroupManager) FindGroupNames(subject string) ([]string, error) {
	var q []string

	query := fmt.Sprintf("SELECT group_id from %s_m WHERE member = ? GROUP BY group_id", m.GetTable())
	if err := m.DB.Select(&q, m.DB.Rebind(query), subject); err != nil {
		return nil, errors.WithStack(err)
	}

	return q, nil
}
