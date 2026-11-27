// Package push owns the students' device push tokens and their notification
// setting, and sends push notifications through the Expo push service.
package push

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	expoTimeout = 10 * time.Second
	pruneEvery  = 24 * time.Hour
)

var _ sender = (*expoClient)(nil)

type sender interface {
	send(ctx context.Context, messages []expoMessage) ([]string, error)
}

type Module struct {
	tx         database.Transactor
	tokens     *tokenRepository
	settings   *settingsRepository
	deliveries *deliveryRepository
	students   *studentRepository
	expo       sender
	runner     *jobs.Runner
}

func New(pool *pgxpool.Pool, accessToken string, runner *jobs.Runner) (Module, error) {
	db := database.New(pool)
	m := Module{
		tx:         db,
		tokens:     newTokenRepository(db),
		settings:   newSettingsRepository(db),
		deliveries: newDeliveryRepository(db),
		students:   newStudentRepository(db),
		expo:       &expoClient{http: &http.Client{Timeout: expoTimeout}, url: expoPushURL, accessToken: accessToken},
		runner:     runner,
	}

	if err := jobs.Register(runner, m.send, jobs.Options{Timeout: time.Minute, MaxAttempts: 5}); err != nil {
		return Module{}, fmt.Errorf("push: %w", err)
	}
	if err := jobs.Register(runner, m.prune, jobs.Options{Timeout: 5 * time.Minute, MaxAttempts: 3}); err != nil {
		return Module{}, fmt.Errorf("push: %w", err)
	}
	if err := jobs.Schedule(runner, "push-prune", PruneArgs{}, pruneEvery); err != nil {
		return Module{}, fmt.Errorf("push: %w", err)
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	tokens := &tokenHandler{push: m}
	settings := &settingsHandler{push: m}

	huma.Register(api, registerMyPushTokenOperation, tokens.registerToken)
	huma.Register(api, deleteMyPushTokenOperation, tokens.deleteToken)
	huma.Register(api, getMyPushSettingsOperation, settings.getSettings)
	huma.Register(api, updateMyPushSettingsOperation, settings.updateSettings)
}
