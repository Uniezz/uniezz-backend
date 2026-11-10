// Package faq stores questions students can read once they are published, and
// proposals that wait for a moderator.
package faq

import (
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	entries *entryRepository
}

func New(pool *pgxpool.Pool) Module {
	return Module{entries: newEntryRepository(database.New(pool))}
}

func (m Module) Register(api huma.API) {
	h := &entryHandler{faq: m}
	huma.Register(api, listEntriesOperation, h.listEntries)
	huma.Register(api, createProposalOperation, h.createProposal)
	huma.Register(api, listProposalsOperation, h.listProposals)
}
