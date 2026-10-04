package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/usos"
)

// Compile-time check: the real USOS client satisfies the interface usosLogin needs.
var _ usosClient = (*usos.Client)(nil)

type fakeUSOS struct {
	rt           usos.RequestToken
	authorizeURL string
	beginErr     error

	user      usos.User
	finishErr error

	beginCalls   int
	finishCalls  int
	finishedWith usos.RequestToken
	verifier     string
}

func (f *fakeUSOS) Begin(_ context.Context) (usos.RequestToken, string, error) {
	f.beginCalls++
	if f.beginErr != nil {
		return usos.RequestToken{}, "", f.beginErr
	}
	rt := f.rt
	return rt, f.authorizeURL, nil
}

func (f *fakeUSOS) Finish(_ context.Context, rt usos.RequestToken, verifier string) (usos.User, error) {
	f.finishCalls++
	f.finishedWith = rt
	f.verifier = verifier
	if f.finishErr != nil {
		return usos.User{}, f.finishErr
	}
	u := f.user
	return u, nil
}

type fakeStateStore struct {
	states  map[string]usosState
	saveErr error
}

func newFakeStateStore() *fakeStateStore {
	return &fakeStateStore{states: map[string]usosState{}}
}

func (s *fakeStateStore) save(_ context.Context, st usosState) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.states[st.RequestToken.Token] = st
	return nil
}

func (s *fakeStateStore) take(_ context.Context, requestToken string) (usosState, error) {
	st, ok := s.states[requestToken]
	if !ok {
		return usosState{}, ErrStateNotFound
	}
	delete(s.states, requestToken)
	return st, nil
}

func ptr[T any](v T) *T {
	return &v
}

func activeStudent() usos.User {
	return usos.User{
		ID:            "123456",
		FirstName:     "Jan",
		LastName:      "Kowalski",
		Email:         ptr("jan@umcs.pl"),
		StudentStatus: ptr(usos.StatusActiveStudent),
	}
}

func newTestUsosLogin() (*usosLogin, *fakeUSOS, *fakeStateStore) {
	client := &fakeUSOS{
		rt:           usos.RequestToken{Token: "req-token", Secret: "req-secret"},
		authorizeURL: "https://apps.umcs.pl/services/oauth/authorize?oauth_token=req-token",
		user:         activeStudent(),
	}
	states := newFakeStateStore()
	login := newUsosLogin(map[UniversityID]usosClient{UniversityUMCS: client}, states)
	return login, client, states
}

func TestParsePlatform(t *testing.T) {
	testCases := []struct {
		input   string
		want    Platform
		wantErr bool
	}{
		{input: "web", want: PlatformWeb},
		{input: "mobile", want: PlatformMobile},
		{input: "", wantErr: true},
		{input: "Web", wantErr: true},
		{input: "desktop", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parsePlatform(tc.input)

			if tc.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("expected ErrInvalidInput, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestUsosLogin_Start(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the authorize URL and remembers the state", func(t *testing.T) {
		login, client, states := newTestUsosLogin()

		url, err := login.start(ctx, UniversityUMCS, PlatformMobile)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if url != client.authorizeURL {
			t.Errorf("expected authorize URL %q, got %q", client.authorizeURL, url)
		}

		st, ok := states.states["req-token"]
		if !ok {
			t.Fatal("expected the state to be saved under the request token")
		}
		want := usosState{RequestToken: client.rt, UniversityID: UniversityUMCS, Platform: PlatformMobile}
		if st != want {
			t.Errorf("saved state mismatch:\nexpected %+v\ngot      %+v", want, st)
		}
	})

	t.Run("rejects an OTP university without calling USOS", func(t *testing.T) {
		login, client, _ := newTestUsosLogin()

		_, err := login.start(ctx, UniversityKUL, PlatformWeb)
		if !errors.Is(err, ErrWrongAuthMethod) {
			t.Fatalf("expected ErrWrongAuthMethod, got: %v", err)
		}
		if client.beginCalls != 0 {
			t.Error("expected USOS not to be called")
		}
	})

	t.Run("rejects an unknown university", func(t *testing.T) {
		login, _, _ := newTestUsosLogin()

		_, err := login.start(ctx, "unknown-uni", PlatformWeb)
		if !errors.Is(err, ErrUnknownUniversity) {
			t.Fatalf("expected ErrUnknownUniversity, got: %v", err)
		}
	})

	t.Run("fails for a USOS university with no configured client", func(t *testing.T) {
		login, _, _ := newTestUsosLogin()

		_, err := login.start(ctx, UniversityUMLub, PlatformWeb)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("USOS failure is ErrUpstream and keeps the cause", func(t *testing.T) {
		login, client, states := newTestUsosLogin()
		cause := errors.New("oauth1: invalid status 401")
		client.beginErr = cause

		_, err := login.start(ctx, UniversityUMCS, PlatformWeb)
		if !errors.Is(err, ErrUpstream) {
			t.Errorf("expected ErrUpstream, got: %v", err)
		}
		if !errors.Is(err, cause) {
			t.Errorf("expected the USOS error to be wrapped so it shows up in logs, got: %v", err)
		}
		if len(states.states) != 0 {
			t.Error("expected no state to be saved")
		}
	})

	t.Run("state store failure is ours, not ErrUpstream, and keeps the cause", func(t *testing.T) {
		login, _, states := newTestUsosLogin()
		cause := errors.New("connection refused")
		states.saveErr = cause

		_, err := login.start(ctx, UniversityUMCS, PlatformWeb)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if errors.Is(err, ErrUpstream) {
			t.Errorf("a database failure must not be reported as provider_unavailable, got: %v", err)
		}
		if !errors.Is(err, cause) {
			t.Errorf("expected the store error to be wrapped, got: %v", err)
		}
	})
}

func TestUsosLogin_Finish(t *testing.T) {
	ctx := context.Background()

	startLogin := func(t *testing.T, login *usosLogin, platform Platform) {
		t.Helper()
		if _, err := login.start(ctx, UniversityUMCS, platform); err != nil {
			t.Fatalf("start failed: %v", err)
		}
	}

	t.Run("returns the identity and platform of an active student", func(t *testing.T) {
		login, client, _ := newTestUsosLogin()
		startLogin(t, login, PlatformMobile)

		id, platform, err := login.finish(ctx, "req-token", "verifier-1")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		want := Identity{
			UniversityID: UniversityUMCS,
			UsosUserID:   "123456",
			Email:        "jan@umcs.pl",
			FirstName:    "Jan",
			LastName:     "Kowalski",
		}
		if id != want {
			t.Errorf("identity mismatch:\nexpected %+v\ngot      %+v", want, id)
		}
		if platform != PlatformMobile {
			t.Errorf("expected platform %q, got %q", PlatformMobile, platform)
		}
		if client.finishedWith != client.rt || client.verifier != "verifier-1" {
			t.Errorf("expected USOS to get the saved request token and the verifier, got %+v and %q",
				client.finishedWith, client.verifier)
		}
	})

	t.Run("a state works only once", func(t *testing.T) {
		login, _, _ := newTestUsosLogin()
		startLogin(t, login, PlatformWeb)

		if _, _, err := login.finish(ctx, "req-token", "verifier-1"); err != nil {
			t.Fatalf("first finish failed: %v", err)
		}

		_, _, err := login.finish(ctx, "req-token", "verifier-1")
		if !errors.Is(err, ErrStateNotFound) {
			t.Fatalf("expected ErrStateNotFound on replay, got: %v", err)
		}
	})

	t.Run("unknown request token does not call USOS", func(t *testing.T) {
		login, client, _ := newTestUsosLogin()

		_, _, err := login.finish(ctx, "never-issued", "verifier-1")
		if !errors.Is(err, ErrStateNotFound) {
			t.Fatalf("expected ErrStateNotFound, got: %v", err)
		}
		if client.finishCalls != 0 {
			t.Error("expected USOS not to be called")
		}
	})

	t.Run("USOS failure is ErrUpstream, keeps the cause and the platform", func(t *testing.T) {
		login, client, _ := newTestUsosLogin()
		startLogin(t, login, PlatformMobile)
		cause := errors.New("usos api returned status: 500")
		client.finishErr = cause

		_, platform, err := login.finish(ctx, "req-token", "verifier-1")
		if !errors.Is(err, ErrUpstream) {
			t.Errorf("expected ErrUpstream, got: %v", err)
		}
		if !errors.Is(err, cause) {
			t.Errorf("expected the USOS error to be wrapped so it shows up in logs, got: %v", err)
		}
		if platform != PlatformMobile {
			t.Errorf("expected platform %q so the handler can redirect with an error, got %q", PlatformMobile, platform)
		}
	})

	for _, status := range []usos.StudentStatus{usos.StatusNotStudent, usos.StatusInactiveStudent} {
		t.Run(fmt.Sprintf("rejects student_status %d", status), func(t *testing.T) {
			login, client, _ := newTestUsosLogin()
			startLogin(t, login, PlatformWeb)
			client.user.StudentStatus = ptr(status)

			_, platform, err := login.finish(ctx, "req-token", "verifier-1")
			if !errors.Is(err, ErrNotStudent) {
				t.Fatalf("expected ErrNotStudent, got: %v", err)
			}
			if platform != PlatformWeb {
				t.Errorf("expected platform %q so the handler can redirect with an error, got %q", PlatformWeb, platform)
			}
		})
	}

	t.Run("null student_status is ErrUpstream, not ErrNotStudent", func(t *testing.T) {
		login, client, _ := newTestUsosLogin()
		startLogin(t, login, PlatformMobile)
		client.user.StudentStatus = nil

		_, platform, err := login.finish(ctx, "req-token", "verifier-1")
		if !errors.Is(err, ErrUpstream) {
			t.Errorf("expected ErrUpstream, got: %v", err)
		}
		if errors.Is(err, ErrNotStudent) {
			t.Errorf("missing access to student_status must not look like a non-student, got: %v", err)
		}
		if platform != PlatformMobile {
			t.Errorf("expected platform %q, got %q", PlatformMobile, platform)
		}
	})

	t.Run("null email gives an identity without email", func(t *testing.T) {
		login, client, _ := newTestUsosLogin()
		startLogin(t, login, PlatformWeb)
		client.user.Email = nil

		id, _, err := login.finish(ctx, "req-token", "verifier-1")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if id.Email != "" {
			t.Errorf("expected empty email, got %q", id.Email)
		}
		if id.UsosUserID != "123456" {
			t.Errorf("expected usos user id 123456, got %q", id.UsosUserID)
		}
	})
}
