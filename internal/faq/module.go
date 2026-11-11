// Package faq stores questions students can read once they are published, and
// proposals that wait for a moderator.
package faq

import (
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	entries    *entryRepository
	publishers map[string]bool
}

func New(pool *pgxpool.Pool, publisherEmails []string) Module {
	publishers := make(map[string]bool, len(publisherEmails))
	for _, email := range publisherEmails {
		publishers[strings.ToLower(email)] = true
	}
	return Module{entries: newEntryRepository(database.New(pool)), publishers: publishers}
}

func (m Module) Register(api huma.API) {
	h := &entryHandler{faq: m}
	huma.Register(api, listEntriesOperation, h.listEntries)
	huma.Register(api, publishEntryOperation, h.publishEntry)
	huma.Register(api, createProposalOperation, h.createProposal)
	huma.Register(api, listProposalsOperation, h.listProposals)
}
