package connections

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestDiscoverCards(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.openTo(t, a, "friends")

	if cards := e.discover(t, a, "mode=friends"); len(cards) != 0 {
		t.Fatalf("b is not open to friends yet, cards = %+v", cards)
	}
	e.openTo(t, b, "friends", "studyBuddy")
	if cards := e.discover(t, a, "mode=studyBuddy"); len(cards) != 0 {
		t.Fatalf("a is not open to studyBuddy, cards = %+v", cards)
	}

	cards := e.discover(t, a, "mode=friends")
	if len(cards) != 1 {
		t.Fatalf("cards = %+v, want b", cards)
	}
	card := cards[0]
	if card.CandidateID == b.id || card.Age == nil || card.UniversityID != "umcs" ||
		len(card.SharedInterests) != 1 || card.SharedInterests[0] != e.tag {
		t.Errorf("card = %+v", card)
	}
	if again := e.discover(t, a, "mode=friends"); len(again) != 1 || again[0].CandidateID != card.CandidateID {
		t.Errorf("candidateId changed between views: %+v", again)
	}
}

func TestDiscoverDatingOnlyAdults(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.openTo(t, a, "dating")

	_, err := e.pool.Exec(context.Background(),
		"INSERT INTO connection_profiles (user_id, open_to) VALUES ($1, ARRAY['dating'])", b.id)
	if err != nil {
		t.Fatal(err)
	}
	if cards := e.discover(t, a, "mode=dating"); len(cards) != 0 {
		t.Fatalf("student without a birth date shown in dating: %+v", cards)
	}

	e.openTo(t, b, "dating")
	if cards := e.discover(t, a, "mode=dating"); len(cards) != 1 {
		t.Fatalf("adult not shown in dating: %+v", cards)
	}
}

func TestDiscoverFacultyFilterHonoursPrivacy(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.openTo(t, a, "studyBuddy")
	e.openTo(t, b, "studyBuddy")

	faculty := "Faculty " + uuid.NewString()
	_, err := e.pool.Exec(context.Background(), `
		INSERT INTO student_programmes (user_id, faculty_name, programme_name, is_primary)
		VALUES ($1, $2, 'Informatyka', true)`, b.id, faculty)
	if err != nil {
		t.Fatal(err)
	}
	query := "mode=studyBuddy&faculty=" + faculty
	if cards := e.discover(t, a, query); len(cards) != 1 {
		t.Fatalf("visible faculty not matched: %+v", cards)
	}

	if _, err := e.pool.Exec(context.Background(), `UPDATE users SET privacy = privacy || '{"showFaculty": false}' WHERE id = $1`, b.id); err != nil {
		t.Fatal(err)
	}
	if cards := e.discover(t, a, query); len(cards) != 0 {
		t.Fatalf("hidden faculty matched the filter: %+v", cards)
	}
}

func TestPassHidesForThirtyDaysFromLatestPass(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.openTo(t, a, "friends")
	e.openTo(t, b, "friends")

	card := e.discover(t, a, "mode=friends")[0]
	decode[ActionResult](t, e.act(a, card.CandidateID, "friends", "pass"), 200)
	if cards := e.discover(t, a, "mode=friends"); len(cards) != 0 {
		t.Fatalf("passed student shown again: %+v", cards)
	}

	agePass := func() {
		t.Helper()
		_, err := e.pool.Exec(context.Background(), `
			UPDATE connection_actions SET created_at = NOW() - INTERVAL '31 days'
			WHERE viewer_id = $1 AND target_id = $2`, a.id, b.id)
		if err != nil {
			t.Fatal(err)
		}
	}
	agePass()
	if cards := e.discover(t, a, "mode=friends"); len(cards) != 1 {
		t.Fatalf("student not shown 31 days after the pass: %+v", cards)
	}

	decode[ActionResult](t, e.act(a, card.CandidateID, "friends", "pass"), 200)
	if cards := e.discover(t, a, "mode=friends"); len(cards) != 0 {
		t.Fatalf("second pass did not hide the student again: %+v", cards)
	}
}

func TestMatch(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.openTo(t, a, "friends")
	e.openTo(t, b, "friends")

	cardOfB := e.discover(t, a, "mode=friends")[0]
	cardOfA := e.discover(t, b, "mode=friends")[0]

	first := decode[ActionResult](t, e.act(b, cardOfA.CandidateID, "friends", "like"), 200)
	if first.Matched || first.Connection != nil {
		t.Fatalf("one like matched: %+v", first)
	}
	second := decode[ActionResult](t, e.act(a, cardOfB.CandidateID, "friends", "like"), 200)
	if !second.Matched || second.Connection == nil || second.Connection.Profile.ID != b.id {
		t.Fatalf("match = %+v", second)
	}

	expectError(t, e.act(b, cardOfB.CandidateID, "friends", "like"), 404, "not_found")
	expectError(t, e.act(a, cardOfB.CandidateID, "friends", "like"), 404, "not_found")
	if cards := e.discover(t, a, "mode=friends"); len(cards) != 0 {
		t.Errorf("connected student shown in discovery: %+v", cards)
	}
}

func TestActChecksThePairAgain(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.openTo(t, a, "friends")
	e.openTo(t, b, "friends")

	cardOfA := e.discover(t, b, "mode=friends")[0]
	cardOfB := e.discover(t, a, "mode=friends")[0]
	decode[ActionResult](t, e.act(a, cardOfB.CandidateID, "friends", "like"), 200)

	if rec := e.api.Post("/blocks/"+b.id.String(), a.bearer); rec.Code != 204 {
		t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
	}
	expectError(t, e.act(b, cardOfA.CandidateID, "friends", "like"), 404, "not_found")

	if rec := e.api.Delete("/blocks/"+b.id.String(), a.bearer); rec.Code != 204 {
		t.Fatalf("unblock: %d %s", rec.Code, rec.Body.String())
	}
	e.openTo(t, a, "studyBuddy")
	expectError(t, e.act(b, cardOfA.CandidateID, "friends", "like"), 404, "not_found")

	var connections int
	err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM connections WHERE user_low = LEAST($1, $2)::uuid AND user_high = GREATEST($1, $2)::uuid", a.id, b.id).Scan(&connections)
	if err != nil {
		t.Fatal(err)
	}
	if connections != 0 {
		t.Errorf("connections = %d, want 0", connections)
	}
}

func TestCrossingLikesAlwaysMatch(t *testing.T) {
	e := newTestEnv(t)
	for range 10 {
		a := e.user(t)
		b := e.user(t)
		e.openTo(t, a, "friends")
		e.openTo(t, b, "friends")
		cardOfB := e.discover(t, a, "mode=friends")[0]
		cardOfA := e.discover(t, b, "mode=friends")[0]

		codes := make(chan int, 2)
		var wg sync.WaitGroup
		wg.Go(func() { codes <- e.act(a, cardOfB.CandidateID, "friends", "like").Code })
		wg.Go(func() { codes <- e.act(b, cardOfA.CandidateID, "friends", "like").Code })
		wg.Wait()
		close(codes)
		for code := range codes {
			if code != 200 {
				t.Fatalf("like status = %d", code)
			}
		}

		var connections int
		err := e.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM connections WHERE user_low = LEAST($1, $2)::uuid AND user_high = GREATEST($1, $2)::uuid", a.id, b.id).Scan(&connections)
		if err != nil {
			t.Fatal(err)
		}
		if connections != 1 {
			t.Fatalf("connections after crossing likes = %d, want 1", connections)
		}
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = ANY($1)", []uuid.UUID{a.id, b.id})
	}
}
