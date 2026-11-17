package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/jovanpet/campus-study-group/internal/httpapi"
	"github.com/jovanpet/campus-study-group/internal/store/storetest"
)

func TestPostgresRepository(t *testing.T) {
	ctx := context.Background()
	tx := storetest.Tx(t)
	repo := NewPostgresRepository(tx)

	name := "Alice"
	u, err := repo.UpsertByAppleSub(ctx, "repo-test-alice", &name, []byte("enc-1"))
	if err != nil {
		t.Fatalf("UpsertByAppleSub: %v", err)
	}
	if u.IsOnboarded() || u.DisplayName == nil || *u.DisplayName != "Alice" || u.School != nil {
		t.Fatalf("new user = %+v, want not onboarded with display name", u)
	}

	// Signing in again: same row, display name kept, refresh token only
	// replaced when a new one is supplied.
	other := "Someone Else"
	again, err := repo.UpsertByAppleSub(ctx, "repo-test-alice", &other, nil)
	if err != nil || again.ID != u.ID || *again.DisplayName != "Alice" {
		t.Fatalf("second upsert = %+v, %v", again, err)
	}
	if enc, _ := repo.AppleRefreshToken(ctx, u.ID); string(enc) != "enc-1" {
		t.Errorf("refresh token = %q, want it kept", enc)
	}
	if _, err := repo.UpsertByAppleSub(ctx, "repo-test-alice", nil, []byte("enc-2")); err != nil {
		t.Fatal(err)
	}
	if enc, _ := repo.AppleRefreshToken(ctx, u.ID); string(enc) != "enc-2" {
		t.Errorf("refresh token = %q, want it replaced", enc)
	}

	schools, err := repo.ListSchools(ctx)
	if err != nil || len(schools) == 0 {
		t.Fatalf("ListSchools = %v, %v", schools, err)
	}
	majors, err := repo.ListMajors(ctx)
	if err != nil || len(majors) == 0 {
		t.Fatalf("ListMajors = %v, %v", majors, err)
	}
	profile := Profile{Username: "alice", DisplayName: "Alice A", SchoolID: schools[0].ID, MajorID: majors[0].ID, Degree: Master}

	done, err := repo.CompleteProfile(ctx, u.ID, profile)
	if err != nil {
		t.Fatalf("CompleteProfile: %v", err)
	}
	if !done.IsOnboarded() || *done.UserName != "alice" || *done.Degree != Master ||
		done.School == nil || done.School.Name != schools[0].Name || done.Major == nil || done.Major.ID != majors[0].ID {
		t.Errorf("onboarded user = %+v", done)
	}

	// Failed statements abort a Postgres transaction, so the error cases each
	// run in a savepoint.
	bob, err := repo.UpsertByAppleSub(ctx, "repo-test-bob", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	failing := map[string]struct {
		p    Profile
		want error
	}{
		"username taken (case-insensitive)": {Profile{"ALICE", "Bob", schools[0].ID, majors[0].ID, PhD}, ErrUsernameTaken},
		"unknown school":                    {Profile{"bob", "Bob", uuid.New(), majors[0].ID, PhD}, ErrInvalid},
	}
	for name, tc := range failing {
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := NewPostgresRepository(sp).CompleteProfile(ctx, bob.ID, tc.p); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
		_ = sp.Rollback(ctx)
	}

	if _, err := repo.CompleteProfile(ctx, uuid.New(), profile); !errors.Is(err, ErrNotFound) {
		t.Errorf("CompleteProfile unknown user: %v", err)
	}
	if _, err := repo.Get(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get unknown user: %v", err)
	}

	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: %v", err)
	}
}

// stubRepo records the profile the service passes through.
type stubRepo struct {
	Repository
	user  User
	saved Profile
	err   error
}

func (s *stubRepo) Get(context.Context, uuid.UUID) (User, error) { return s.user, s.err }

func (s *stubRepo) CompleteProfile(_ context.Context, _ uuid.UUID, p Profile) (User, error) {
	s.saved = p
	return s.user, s.err
}

func TestCompleteProfileValidation(t *testing.T) {
	ctx := context.Background()
	valid := Profile{Username: "  Alice_01 ", DisplayName: " Alice ", SchoolID: uuid.New(), MajorID: uuid.New(), Degree: Bachelor}

	repo := &stubRepo{}
	if _, err := NewService(repo).CompleteProfile(ctx, uuid.New(), valid); err != nil {
		t.Fatalf("valid profile: %v", err)
	}
	if repo.saved.Username != "alice_01" || repo.saved.DisplayName != "Alice" {
		t.Errorf("saved = %+v, want normalised username and trimmed name", repo.saved)
	}

	invalid := map[string]func(*Profile){
		"short username":   func(p *Profile) { p.Username = "al" },
		"long username":    func(p *Profile) { p.Username = strings.Repeat("a", 21) },
		"username symbols": func(p *Profile) { p.Username = "alice smith" },
		"empty name":       func(p *Profile) { p.DisplayName = "   " },
		"long name":        func(p *Profile) { p.DisplayName = strings.Repeat("é", 51) },
		"bad degree":       func(p *Profile) { p.Degree = "postdoc" },
		"no school":        func(p *Profile) { p.SchoolID = uuid.Nil() },
		"no major":         func(p *Profile) { p.MajorID = uuid.Nil() },
	}
	for name, mutate := range invalid {
		p := valid
		mutate(&p)
		if _, err := NewService(&stubRepo{}).CompleteProfile(ctx, uuid.New(), p); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

// asUser stands in for auth.RequireUser: it authenticates every request as id.
// This is how handler tests in any package avoid real sign-in.
func asUser(id uuid.UUID) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(httpapi.WithUserID(r.Context(), id)))
		})
	}
}

func TestHandler(t *testing.T) {
	id := uuid.New()
	repo := &stubRepo{user: User{ID: id}}
	mux := http.NewServeMux()
	NewHandler(NewService(repo), asUser(id)).RegisterRoutes(mux)

	request := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}

	rec := request("GET", "/me", "")
	var me struct {
		ID        uuid.UUID `json:"id"`
		Username  *string   `json:"username"`
		Onboarded *bool     `json:"onboarded"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET /me = %d %s (%v)", rec.Code, rec.Body, err)
	}
	if me.ID != id || me.Username != nil || me.Onboarded == nil || *me.Onboarded {
		t.Errorf("GET /me = %s, want id, null username, onboarded false", rec.Body)
	}

	body := `{"username":"alice","display_name":"Alice","school_id":"` + uuid.New().String() +
		`","major_id":"` + uuid.New().String() + `","degree":"graduate"}`
	if rec := request("PUT", "/me/profile", body); rec.Code != http.StatusOK {
		t.Errorf("PUT /me/profile = %d %s", rec.Code, rec.Body)
	}
	if repo.saved.Username != "alice" || repo.saved.Degree != Master {
		t.Errorf("saved profile = %+v", repo.saved)
	}
	if rec := request("PUT", "/me/profile", `{"username":"a"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid profile = %d, want 400", rec.Code)
	}
	if rec := request("PUT", "/me/profile", `{"school_id":"not-a-uuid"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed uuid = %d, want 400", rec.Code)
	}

	repo.err = ErrUsernameTaken
	if rec := request("PUT", "/me/profile", body); rec.Code != http.StatusConflict {
		t.Errorf("taken username = %d, want 409", rec.Code)
	}
	repo.err = ErrNotFound
	if rec := request("GET", "/me", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("deleted user = %d, want 401", rec.Code)
	}
}
