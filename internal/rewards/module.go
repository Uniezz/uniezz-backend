// Package rewards owns Uniezz coins: the ledger of grants and each student's balance. Other modules grant coins for what students do; nothing spends them yet.
package rewards

import (
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	tx     database.Transactor
	coins  *coinRepository
	quests *questRepository
}

func New(pool *pgxpool.Pool) Module {
	db := database.New(pool)
	return Module{tx: db, coins: newCoinRepository(db), quests: newQuestRepository(db)}
}
