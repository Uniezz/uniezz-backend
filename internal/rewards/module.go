// Package rewards owns Uniezz coins: the ledger of grants and each student's balance, and the daily quests and admin-made challenges that pay them. Other modules grant coins and count actions; nothing spends coins yet.
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
	tx         database.Transactor
	coins      *coinRepository
	quests     *questRepository
	challenges *challengeRepository
}

func New(pool *pgxpool.Pool, runner *jobs.Runner) (Module, error) {
	db := database.New(pool)
	m := Module{tx: db, coins: newCoinRepository(db), quests: newQuestRepository(db), challenges: newChallengeRepository(db)}
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

	challenges := &challengeHandler{rewards: m}
	huma.Register(api, listMyChallengesOperation, challenges.listMyChallenges)
	huma.Register(api, listChallengesOperation, challenges.listChallenges)
	huma.Register(api, createChallengeOperation, challenges.createChallenge)
	huma.Register(api, getChallengeOperation, challenges.getChallenge)
	huma.Register(api, updateChallengeOperation, challenges.updateChallenge)
	huma.Register(api, archiveChallengeOperation, challenges.archiveChallenge)
}
