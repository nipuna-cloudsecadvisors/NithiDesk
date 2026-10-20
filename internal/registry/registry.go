// Package registry holds people and organisations, one record per real-world
// entity (doc 04 §1). Client, party, witness are roles an entity plays
// elsewhere. M1 is thin: kind plus a name in one or both scripts, with the
// search shadows built by internal/platform/textnorm. Deduplication, merge,
// identifiers and relationships arrive in M2.
package registry

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/authz"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/db"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/textnorm"
)

// Entity kinds.
const (
	KindPerson       = "person"
	KindOrganisation = "organisation"
	KindGovernment   = "government_body"
)

// Name is a bilingual entity name.
type Name struct {
	EN *string `json:"en,omitempty"`
	SI *string `json:"si,omitempty"`
	TA *string `json:"ta,omitempty"`
}

// Entity is a registry record as pickers see it.
type Entity struct {
	ID    uuid.UUID `json:"id"`
	Kind  string    `json:"kind"`
	Name  Name      `json:"name"`
	Score float64   `json:"score,omitempty"`
	Match string    `json:"match,omitempty"` // exact, strong, possible
}

// Input creates an entity.
type Input struct {
	Kind   string `json:"kind"`
	NameEN string `json:"name_en"`
	NameSI string `json:"name_si"`
}

// Validate returns field errors for in.
func (in Input) Validate(prefix string) []apperr.FieldError {
	var fields []apperr.FieldError
	switch in.Kind {
	case KindPerson, KindOrganisation, KindGovernment:
	default:
		fields = append(fields, apperr.Field(prefix+"kind", "invalid"))
	}
	if strings.TrimSpace(in.NameEN) == "" && strings.TrimSpace(in.NameSI) == "" {
		fields = append(fields, apperr.Field(prefix+"name_en", "required"))
	}
	if len(in.NameEN) > 300 || len(in.NameSI) > 600 {
		fields = append(fields, apperr.Field(prefix+"name_en", "too_long"))
	}
	return fields
}

// Create inserts an entity inside tx and returns its id. createdVia is
// user, import or system.
func Create(ctx context.Context, tx pgx.Tx, firmID, by uuid.UUID, in Input, createdVia string) (uuid.UUID, error) {
	en := strings.TrimSpace(textnorm.Normalise(in.NameEN, true))
	si := strings.TrimSpace(textnorm.Normalise(in.NameSI, true))
	sh := textnorm.NameShadows(en, si)
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		insert into entity (firm_id, kind, name_en, name_si, name_norm, name_latin, name_key, name_skeleton, initials,
		                    textnorm_rules, created_by, created_via)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) returning id`,
		firmID, in.Kind, nullable(en), nullable(si), sh.Norm, sh.Latin, sh.Key, sh.Skeleton, sh.Initials,
		textnorm.RulesVersion, nullUUID(by), createdVia).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create entity: %w", err)
	}
	return id, nil
}

// FindExact returns an existing entity whose normalised name equals one of
// in's names exactly, so an import does not create a duplicate per row.
// It never merges near-matches; that is a human decision (M2).
func FindExact(ctx context.Context, tx pgx.Tx, in Input) (uuid.UUID, bool, error) {
	sh := textnorm.NameShadows(in.NameEN, in.NameSI)
	if sh.Norm == "" {
		return uuid.Nil, false, nil
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `select id from entity where kind = $1 and name_norm = $2 and merged_into_id is null
		order by created_at limit 1`, in.Kind, sh.Norm).Scan(&id)
	if err == pgx.ErrNoRows {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("find entity: %w", err)
	}
	return id, true, nil
}

// Load returns entities by id inside tx.
func Load(ctx context.Context, tx pgx.Tx, ids []uuid.UUID) (map[uuid.UUID]Entity, error) {
	out := make(map[uuid.UUID]Entity, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `select id, kind, name_en, name_si, name_ta from entity where id = any($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("load entities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e Entity
		if err := rows.Scan(&e.ID, &e.Kind, &e.Name.EN, &e.Name.SI, &e.Name.TA); err != nil {
			return nil, fmt.Errorf("load entities: %w", err)
		}
		out[e.ID] = e
	}
	return out, rows.Err()
}

// Service exposes registry search and creation.
type Service struct {
	pool *pgxpool.Pool
}

// New builds the service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// CreateEntity creates an entity (entity.create).
func (s *Service) CreateEntity(ctx context.Context, p *authz.Principal, in Input) (*Entity, error) {
	if err := p.Require(authz.EntityCreate); err != nil {
		return nil, err
	}
	if fields := in.Validate(""); len(fields) > 0 {
		return nil, apperr.Invalid(fields...)
	}
	var out Entity
	err := db.InTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		id, err := Create(ctx, tx, p.FirmID, p.UserID, in, "user")
		if err != nil {
			return err
		}
		m, err := Load(ctx, tx, []uuid.UUID{id})
		out = m[id]
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Search finds entities by name in either script. Candidates come from
// pg_trgm word similarity on the Latin shadows; they are re-ranked with the
// structured name score and bucketed (spike #25 §5). Only callers who may
// create entities (and so pick clients and parties) can search: the registry
// reveals whom the firm knows, so it is not open to every role.
func (s *Service) Search(ctx context.Context, p *authz.Principal, q string, limit int) ([]Entity, error) {
	if err := p.Require(authz.EntityCreate); err != nil {
		return nil, err
	}
	q = strings.TrimSpace(q)
	if len([]rune(q)) < 2 {
		return []Entity{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	latin := textnorm.LatinText(q)
	key := textnorm.QueryKey(q)
	var out []Entity
	err := db.ReadTx(ctx, s.pool, p.Scope(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			select id, kind, name_en, name_si, name_ta
			from entity
			where merged_into_id is null
			  and ($1 <% name_latin or $2 <% name_key or name_latin ilike '%' || $1 || '%')
			order by greatest(word_similarity($1, name_latin), word_similarity($2, name_key)) desc
			limit 200`, latin, key)
		if err != nil {
			return fmt.Errorf("search entities: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var e Entity
			if err := rows.Scan(&e.ID, &e.Kind, &e.Name.EN, &e.Name.SI, &e.Name.TA); err != nil {
				return fmt.Errorf("search entities: %w", err)
			}
			e.Score = bestScore(q, e.Name)
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	normQ := strings.ToLower(textnorm.Normalise(q, false))
	for i := range out {
		switch {
		case matchesExactly(normQ, out[i].Name):
			out[i].Match, out[i].Score = "exact", 1
		case out[i].Score >= textnorm.StrongThreshold:
			out[i].Match = "strong"
		case out[i].Score >= textnorm.PossibleThreshold:
			out[i].Match = "possible"
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func bestScore(q string, n Name) float64 {
	best := 0.0
	for _, v := range []*string{n.EN, n.SI, n.TA} {
		if v != nil {
			best = max(best, textnorm.ScoreNames(q, *v))
		}
	}
	return best
}

func matchesExactly(normQ string, n Name) bool {
	for _, v := range []*string{n.EN, n.SI, n.TA} {
		if v != nil && strings.ToLower(textnorm.Normalise(*v, false)) == normQ {
			return true
		}
	}
	return false
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
