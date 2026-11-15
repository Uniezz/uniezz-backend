// Package rewards owns Uniezz coins: the ledger of grants and each student's balance. Other modules grant coins for what students do; nothing spends them yet.
package rewards

import (
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pruneQuestsEvery = 24 * time.Hour

type Module struct {
	tx     database.Transactor
	coins  *coinRepository
	quests *questRepository
}

func New(pool *pgxpool.Pool, runner *jobs.Runner) (Module, error) {
	db := database.New(pool)
	m := Module{tx: db, coins: newCoinRepository(db), quests: newQuestRepository(db)}
	if err := jobs.Register(runner, m.pruneQuests, jobs.Options{Timeout: time.Minute, MaxAttempts: 3}); err != nil {
		return Module{}, fmt.Errorf("rewards: %w", err)
	}
	if err := jobs.Schedule(runner, "rewards-prune-quests", PruneQuestsArgs{}, pruneQuestsEvery); err != nil {
		return Module{}, fmt.Errorf("rewards: %w", err)
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	quests := &questHandler{rewards: m}
	grants := &grantHandler{rewards: m}
	huma.Register(api, getMyDailyQuestsOperation, quests.getMyDailyQuests)
	huma.Register(api, listMyCoinGrantsOperation, grants.listMyCoinGrants)
}
