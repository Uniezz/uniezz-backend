// Package faq stores questions students can read once they are published, and
// proposals that wait for a moderator.
package faq

import (
	"context"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ rewardService = rewards.Module{}

type rewardService interface {
	Grant(ctx context.Context, userID uuid.UUID, reason rewards.Reason, refID, actorID *uuid.UUID) (int, error)
}

type Module struct {
	tx         database.Transactor
	entries    *entryRepository
	publishers map[string]bool
	rewards    rewardService
}

func New(pool *pgxpool.Pool, publisherEmails []string, rewardsModule rewardService) Module {
	publishers := make(map[string]bool, len(publisherEmails))
	for _, email := range publisherEmails {
		publishers[strings.ToLower(email)] = true
	}
	db := database.New(pool)
	return Module{tx: db, entries: newEntryRepository(db), publishers: publishers, rewards: rewardsModule}
}

func (m Module) Register(api huma.API) {
	h := &entryHandler{faq: m}
	huma.Register(api, listEntriesOperation, h.listEntries)
	huma.Register(api, publishEntryOperation, h.publishEntry)
	huma.Register(api, createProposalOperation, h.createProposal)
	huma.Register(api, listProposalsOperation, h.listProposals)

	approvals := &approvalHandler{faq: m}
	huma.Register(api, listPendingProposalsOperation, approvals.listPendingProposals)
	huma.Register(api, approveProposalOperation, approvals.approveProposal)
}
