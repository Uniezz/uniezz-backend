package rewards

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func grantRows(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM coin_grants WHERE user_id = $1", userID).Scan(&n); err != nil {
		t.Fatalf("count grants: %v", err)
	}
	return n
}

func TestGrant(t *testing.T) {
	pool := setupTestDB(t)
	m := newModule(t, pool)
	ctx := context.Background()

	t.Run("a student with no grants has a balance of zero", func(t *testing.T) {
		if got, err := m.Balance(ctx, newUser(t, pool)); err != nil || got != 0 {
			t.Errorf("balance = %d, %v; want 0", got, err)
		}
	})

	t.Run("the same action pays once and reports it", func(t *testing.T) {
		user := newUser(t, pool)
		for i, want := range []int{100, 0} {
			paid, err := m.Grant(ctx, user, ReasonWelcome, nil, nil)
			if err != nil || paid != want {
				t.Fatalf("grant %d: paid %v, %v; want %v", i, paid, err, want)
			}
		}
		if got, _ := m.Balance(ctx, user); got != 100 || grantRows(t, pool, user) != 1 {
			t.Errorf("balance %d, rows %d; want 100, 1", got, grantRows(t, pool, user))
		}
	})

	t.Run("each liker of a post pays the author once and a new post pays again", func(t *testing.T) {
		author, post, otherPost := newUser(t, pool), uuid.New(), uuid.New()
		first, second := newUser(t, pool), newUser(t, pool)
		grants := []struct{ ref, actor *uuid.UUID }{{&post, &first}, {&post, &first}, {&post, &second}, {&otherPost, &first}}
		for _, g := range grants {
			if _, err := m.Grant(ctx, author, ReasonLikeReceived, g.ref, g.actor); err != nil {
				t.Fatalf("grant: %v", err)
			}
		}
		if got, _ := m.Balance(ctx, author); got != 3 {
			t.Errorf("balance = %d, want 3", got)
		}
	})

	t.Run("concurrent grants of one action pay once", func(t *testing.T) {
		user, post := newUser(t, pool), uuid.New()
		var wg sync.WaitGroup
		errs := make([]error, 10)
		for i := range errs {
			wg.Go(func() { _, errs[i] = m.Grant(ctx, user, ReasonComment, &post, nil) })
		}
		wg.Wait()
		if err := errors.Join(errs...); err != nil {
			t.Fatalf("grant: %v", err)
		}
		if got, _ := m.Balance(ctx, user); got != 1 || grantRows(t, pool, user) != 1 {
			t.Errorf("balance %d, rows %d; want 1, 1", got, grantRows(t, pool, user))
		}
	})

	t.Run("a grant inside a rolled-back transaction pays nothing", func(t *testing.T) {
		user := newUser(t, pool)
		rollback := errors.New("rollback")
		err := database.New(pool).WithTx(ctx, func(ctx context.Context) error {
			if _, err := m.Grant(ctx, user, ReasonBirthDate, nil, nil); err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatalf("tx err = %v", err)
		}
		if got, _ := m.Balance(ctx, user); got != 0 || grantRows(t, pool, user) != 0 {
			t.Errorf("balance %d, rows %d; want 0, 0", got, grantRows(t, pool, user))
		}
	})

	t.Run("an unknown reason is an error and pays nothing", func(t *testing.T) {
		user := newUser(t, pool)
		if _, err := m.Grant(ctx, user, Reason("bogus"), nil, nil); err == nil {
			t.Error("want an error")
		}
		if n := grantRows(t, pool, user); n != 0 {
			t.Errorf("rows = %d, want 0", n)
		}
	})
}

func TestReasonEnumListsEveryReason(t *testing.T) {
	field, _ := reflect.TypeFor[CoinGrant]().FieldByName("Reason")
	listed := strings.Split(field.Tag.Get("enum"), ",")
	slices.Sort(listed)

	var reasons []string
	for r := range amounts {
		reasons = append(reasons, string(r))
	}
	slices.Sort(reasons)

	if !slices.Equal(listed, reasons) {
		t.Errorf("CoinGrant.reason enum = %v, want every reason that pays %v", listed, reasons)
	}
}
