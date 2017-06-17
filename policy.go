package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	. "github.com/ory/ladon"
	"github.com/ory/ladon/compiler"
	"github.com/pkg/errors"
)

var policySchemas = func(table string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE %s_p (
		ID           varchar(255) NOT NULL,
		DESCRIPTION  VARCHAR2 (4000) NULL,
		EFFECT       VARCHAR2 (4000) NOT NULL,
		CONDITIONS 	 long raw NULL,
		CONSTRAINT %[1]s_p_pk_idx PRIMARY KEY (ID)
	)`, table),
		// subject
		fmt.Sprintf(`CREATE TABLE %s_s (
		ID          varchar(64) NOT NULL,
		HAS_REGEX   CHAR(1 BYTE) NOT NULL,
		COMPILED 	varchar(511) NOT NULL UNIQUE,
		TEMPLATE 	varchar(511) NOT NULL UNIQUE,
		CONSTRAINT %[1]s_s_pk_idx PRIMARY KEY (ID)
	)`, table),
		// action
		fmt.Sprintf(`CREATE TABLE %s_a (
		ID       	varchar(64) NOT NULL,
		HAS_REGEX   CHAR(1 BYTE) NOT NULL,
		COMPILED 	varchar(511) NOT NULL UNIQUE,
		TEMPLATE 	varchar(511) NOT NULL UNIQUE,
		CONSTRAINT %[1]s_a_pk_idx PRIMARY KEY (ID)
	)`, table),
		// resource
		fmt.Sprintf(`CREATE TABLE %s_r (
		ID       	varchar(64) NOT NULL,
		HAS_REGEX   CHAR(1 BYTE) NOT NULL,
		COMPILED 	varchar(511) NOT NULL UNIQUE,
		TEMPLATE 	varchar(511) NOT NULL UNIQUE,
		CONSTRAINT %[1]s_r_pk_idx PRIMARY KEY (ID)
	)`, table),
		// subject to policy
		fmt.Sprintf(`CREATE TABLE %[1]s_sr (
		POLICY 		varchar(255) NOT NULL,
		SUBJECT 	varchar(64) NOT NULL,

		CONSTRAINT %[1]s_sr_pk_idx PRIMARY KEY (POLICY, SUBJECT),
		CONSTRAINT %[1]s_srp_fk FOREIGN KEY (POLICY) REFERENCES %[1]s_p (ID) ON DELETE CASCADE,
		CONSTRAINT %[1]s_srs_fk FOREIGN KEY (SUBJECT) REFERENCES %[1]s_s (ID) ON DELETE CASCADE
	)`, table),
		// action to policy
		fmt.Sprintf(`CREATE TABLE %[1]s_ar (
		POLICY 		varchar(255) NOT NULL,
		ACTION_ID 		varchar(64) NOT NULL,

		CONSTRAINT %[1]s_ar_pk_idx PRIMARY KEY (POLICY, ACTION_ID),
		CONSTRAINT %[1]s_arp_fk FOREIGN KEY (POLICY) REFERENCES %[1]s_p (ID) ON DELETE CASCADE,
		CONSTRAINT %[1]s_ara_fk FOREIGN KEY (ACTION_ID) REFERENCES %[1]s_a (ID) ON DELETE CASCADE
	)`, table),
		// resource to policy
		fmt.Sprintf(`CREATE TABLE %[1]s_rr (
		POLICY 		varchar(255) NOT NULL,
		RESOURCE_ID	varchar(64) NOT NULL,

		CONSTRAINT %[1]s_rr_pk_idx PRIMARY KEY (POLICY, RESOURCE_ID),
		CONSTRAINT %[1]s_rrp_fk FOREIGN KEY (POLICY) REFERENCES %[1]s_p (ID) ON DELETE CASCADE,
		CONSTRAINT %[1]s_rrr_fk FOREIGN KEY (RESOURCE_ID) REFERENCES %[1]s_r (ID) ON DELETE CASCADE
	)`, table),
	}
}

// PolicyManager is a postgres implementation for Manager to store policies persistently.
type PolicyManager struct {
	DB    *sqlx.DB
	Table string
}

func (s *PolicyManager) GetTable() string {
	if s.Table == "" {
		return "hydpol"
	}
	return s.Table
}

func (s *PolicyManager) CreateSchemas() (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, errors.WithStack(err)
	}

	for _, schema := range policySchemas(s.GetTable()) {
		if _, err := tx.Exec(schema); err != nil {
			if err := tx.Rollback(); err != nil {
				return 0, errors.WithStack(err)
			}
			return 0, errors.Wrapf(err, "Could not migrate policy sql schema: %s", schema)
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

// Create inserts a new policy
func (s *PolicyManager) Create(policy Policy) (err error) {
	conditions := []byte("{}")
	if policy.GetConditions() != nil {
		cs := policy.GetConditions()
		conditions, err = json.Marshal(&cs)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return errors.WithStack(err)
	}

	query := fmt.Sprintf("INSERT /*+ IGNORE_ROW_ON_DUPKEY_INDEX (%[1]s_p, %[1]s_p_pk_idx ) */ INTO %[1]s_p (ID, DESCRIPTION, EFFECT, CONDITIONS) VALUES (?, ?, ?, ?)", s.GetTable())
	if _, err = tx.Exec(s.DB.Rebind(query), policy.GetID(), policy.GetDescription(), policy.GetEffect(), conditions); err != nil {
		if err := tx.Rollback(); err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(err)
	}

	type relation struct {
		p []string
		t string
		c string
	}
	var relations = []relation{{p: policy.GetActions(), t: "a", c: "ACTION_ID"}, {p: policy.GetResources(), t: "r", c: "RESOURCE_ID"}, {p: policy.GetSubjects(), t: "s", c: "SUBJECT"}}

	for _, v := range relations {
		for _, template := range v.p {
			h := sha256.New()
			h.Write([]byte(template))
			id := fmt.Sprintf("%x", h.Sum(nil))

			compiled, err := compiler.CompileRegex(template, policy.GetStartDelimiter(), policy.GetEndDelimiter())
			if err != nil {
				if err := tx.Rollback(); err != nil {
					return errors.WithStack(err)
				}
				return errors.WithStack(err)
			}

			if _, err := tx.Exec(s.DB.Rebind(fmt.Sprintf("INSERT /*+ IGNORE_ROW_ON_DUPKEY_INDEX (%[1]s_%[2]s, %[1]s_%[2]s_pk_idx) */ INTO %[1]s_%[2]s (ID, TEMPLATE, COMPILED, HAS_REGEX) VALUES (?, ?, ?, ?)", s.GetTable(), v.t)), id, template, compiled.String(), strings.Index(template, string(policy.GetStartDelimiter())) > -1); err != nil {
				if err := tx.Rollback(); err != nil {
					return errors.WithStack(err)
				}
				return errors.WithStack(err)
			}

			if _, err := tx.Exec(s.DB.Rebind(fmt.Sprintf("INSERT /*+ IGNORE_ROW_ON_DUPKEY_INDEX (%[1]s_%[2]sr, %[1]s_%[2]sr_pk_idx) */ INTO %[1]s_%[2]sr (POLICY, %[3]s) VALUES (?, ?)", s.GetTable(), v.t, v.c)), policy.GetID(), id); err != nil {
				if err := tx.Rollback(); err != nil {
					return errors.WithStack(err)
				}
				return errors.WithStack(err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		if err := tx.Rollback(); err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(err)
	}

	return nil
}

func (s *PolicyManager) FindRequestCandidates(r *Request) (Policies, error) {
	var query string = fmt.Sprintf(`SELECT
	p.ID as ID, p.EFFECT as EFFECT, p.CONDITIONS as CONDITIONS, p.DESCRIPTION as DESCRIPTION, tsubject.TEMPLATE as "SUBJECT", tresource.TEMPLATE as "RESOURCE", taction.TEMPLATE as "ACTION"
FROM
	%[1]s_p p

INNER JOIN %[1]s_sr rs ON rs.POLICY = p.ID
LEFT JOIN %[1]s_ar ra ON ra.POLICY = p.ID
LEFT JOIN %[1]s_rr rr ON rr.POLICY = p.ID

INNER JOIN %[1]s_s tsubject ON rs.SUBJECT = tsubject.ID
LEFT JOIN %[1]s_a taction ON ra.ACTION_ID = taction.ID
LEFT JOIN %[1]s_r tresource ON rr.RESOURCE_ID = tresource.ID

WHERE

( tsubject.HAS_REGEX = 0 AND tsubject.TEMPLATE = ? )
OR
( tsubject.HAS_REGEX = 1 AND REGEXP_LIKE (?, tsubject.COMPILED) )
`, s.GetTable())

	rows, err := s.DB.Query(s.DB.Rebind(query), r.Subject, r.Subject)
	if err == sql.ErrNoRows {
		return nil, NewErrResourceNotFound(err)
	} else if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()

	return scanRows(rows)
}

func scanRows(rows *sql.Rows) (Policies, error) {
	var policies = map[string]*DefaultPolicy{}

	for rows.Next() {
		var p DefaultPolicy
		var conditions []byte
		var resource, subject, action sql.NullString
		p.Actions = []string{}
		p.Subjects = []string{}
		p.Resources = []string{}

		if err := rows.Scan(&p.ID, &p.Effect, &conditions, &p.Description, &subject, &resource, &action); err == sql.ErrNoRows {
			return nil, NewErrResourceNotFound(err)
		} else if err != nil {
			return nil, errors.WithStack(err)
		}

		p.Conditions = Conditions{}
		if err := json.Unmarshal(conditions, &p.Conditions); err != nil {
			return nil, errors.WithStack(err)
		}

		if c, ok := policies[p.ID]; ok {
			if action.Valid && action.String != "" {
				policies[p.ID].Actions = append(c.Actions, action.String)
			}

			if subject.Valid&& subject.String != ""  {
				policies[p.ID].Subjects = append(c.Subjects, subject.String)
			}

			if resource.Valid && resource.String != "" {
				policies[p.ID].Resources = append(c.Resources, resource.String)
			}
		} else {
			if action.Valid && action.String != "" {
				p.Actions = []string{action.String}
			}

			if subject.Valid&& subject.String != ""  {
				p.Subjects = []string{subject.String}
			}

			if resource.Valid && resource.String != "" {
				p.Resources = []string{resource.String}
			}

			policies[p.ID] = &p
		}
	}

	var result = make(Policies, len(policies))
	var count int
	for _, v := range policies {
		v.Actions = uniq(v.Actions)
		v.Resources = uniq(v.Resources)
		v.Subjects = uniq(v.Subjects)
		result[count] = v
		count++
	}

	return result, nil
}

var policyGetAllQuery = func(table string) string {
	return fmt.Sprintf(`SELECT
		p.ID, p.EFFECT, p.CONDITIONS, p.DESCRIPTION,
		tsubject.TEMPLATE "SUBJECT", tresource.TEMPLATE "RESOURCE", taction.TEMPLATE "ACTION"
	FROM
		%[1]s_p p

	LEFT JOIN %[1]s_sr rs ON rs.POLICY = p.ID
	LEFT JOIN %[1]s_ar ra ON ra.POLICY = p.ID
	LEFT JOIN %[1]s_rr rr ON rr.POLICY = p.ID

	LEFT JOIN %[1]s_s tsubject ON rs.SUBJECT = tsubject.ID
	LEFT JOIN %[1]s_a taction ON ra.ACTION_ID = taction.ID
	LEFT JOIN %[1]s_r tresource ON rr.RESOURCE_ID = tresource.ID

`, table)
}

// GetAll returns all policies
func (s *PolicyManager) GetAll(limit, offset int64) (Policies, error) {
	query := s.DB.Rebind(policyGetAllQuery(s.GetTable()))

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()

	pols, err := scanRows(rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if offset + limit > int64(len(pols)) {
		limit = int64(len(pols))
		offset = 0
	}

	return pols[offset:limit], nil
}

// Get retrieves a policy.
func (s *PolicyManager) Get(id string) (Policy, error) {
	query := s.DB.Rebind(policyGetAllQuery(s.GetTable()) + "WHERE p.ID=?")

	rows, err := s.DB.Query(query, id)
	if err == sql.ErrNoRows {
		return nil, NewErrResourceNotFound(err)
	} else if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()

	policies, err := scanRows(rows)
	if err != nil {
		return nil, errors.WithStack(err)
	} else if len(policies) == 0 {
		return nil, NewErrResourceNotFound(sql.ErrNoRows)
	}

	return policies[0], nil
}

// Delete removes a policy.
func (s *PolicyManager) Delete(id string) error {
	query := fmt.Sprintf("DELETE FROM %s_p WHERE ID=?", s.GetTable())
	_, err := s.DB.Exec(s.DB.Rebind(query), id)
	return errors.WithStack(err)
}

func uniq(input []string) []string {
	u := make([]string, 0, len(input))
	m := make(map[string]bool)

	for _, val := range input {
		if _, ok := m[val]; !ok {
			m[val] = true
			u = append(u, val)
		}
	}

	return u
}
