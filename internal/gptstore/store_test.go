package gptstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-maid/internal/gptsession"
)

func sampleAccount(id, email string) *Account {
	return &Account{
		ID:           id,
		Label:        email,
		Email:        email,
		AccountID:    id,
		PlanType:     "plus",
		SourceFormat: gptsession.SourceWebSession,
		SourceName:   "test.json",
		ImportedAt:   time.Unix(0, 0).UTC(),
		Refreshable:  true,
		Tokens:       gptsession.Tokens{AccessToken: "at-" + id, RefreshToken: "rt-" + id},
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	st, err := Load(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(st.Data.Accounts) != 0 || st.Data.Version != 1 {
		t.Errorf("unexpected empty store: %+v", st.Data)
	}
}

func TestLoadCorruptIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for a corrupt store")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Upsert(sampleAccount("acc-1", "a@example.com"))
	st.Data.ActiveID = "acc-1"
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("store perm = %o, want 600", perm)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Data.Accounts) != 1 || reloaded.Data.Accounts[0].Email != "a@example.com" {
		t.Errorf("roundtrip mismatch: %+v", reloaded.Data)
	}
	if reloaded.Data.ActiveID != "acc-1" || reloaded.Active() == nil {
		t.Errorf("active id lost: %+v", reloaded.Data)
	}
}

func TestUpsertReplacesSameID(t *testing.T) {
	st := &Store{Data: &Data{Version: 1}}
	st.Upsert(sampleAccount("acc-1", "old@example.com"))
	updated := sampleAccount("acc-1", "new@example.com")
	st.Upsert(updated)
	if len(st.Data.Accounts) != 1 || st.Data.Accounts[0].Email != "new@example.com" {
		t.Errorf("upsert did not replace: %+v", st.Data.Accounts)
	}
}

func TestFind(t *testing.T) {
	st := &Store{Data: &Data{Version: 1}}
	st.Upsert(sampleAccount("acc-aaaa", "alpha@example.com"))
	st.Upsert(sampleAccount("acc-bbbb", "beta@example.com"))

	if account, index := st.Find("2"); account == nil || index != 1 {
		t.Errorf("Find(2) = %v, %d", account, index)
	}
	if account, _ := st.Find("beta@example.com"); account == nil {
		t.Error("Find(email) failed")
	}
	if account, _ := st.Find("acc-bbbb"); account == nil {
		t.Error("Find(account_id) failed")
	}
	if account, _ := st.Find("acc-bb"); account == nil {
		t.Error("Find(prefix) failed")
	}
	if account, _ := st.Find("acc-"); account != nil {
		t.Error("ambiguous prefix should not resolve")
	}
	if account, _ := st.Find("missing"); account != nil {
		t.Error("Find(missing) should fail")
	}
}

func TestRemoveClearsActive(t *testing.T) {
	st := &Store{Data: &Data{Version: 1}}
	st.Upsert(sampleAccount("acc-1", "a@example.com"))
	st.Upsert(sampleAccount("acc-2", "b@example.com"))
	st.Data.ActiveID = "acc-1"
	removed, ok := st.Remove("1")
	if !ok || removed.AccountID != "acc-1" {
		t.Fatalf("Remove(1) = %v, %v", removed, ok)
	}
	if st.Data.ActiveID != "" {
		t.Errorf("active id not cleared: %q", st.Data.ActiveID)
	}
}

func TestFromSessionKeysOnAccountID(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	converted := gptsession.Account{
		Label: "mark@example.com", Email: "mark@example.com", AccountID: "ACC-UUID",
		SourceFormat: gptsession.SourceWebSession,
		Tokens:       gptsession.Tokens{AccessToken: "tok"},
	}
	account := FromSession(converted, now)
	if account.ID != "acc-uuid" {
		t.Errorf("ID = %q, want acc-uuid (lowercased account id)", account.ID)
	}

	noID := converted
	noID.AccountID = ""
	if got := FromSession(noID, now).ID; got != "mark@example.com" {
		t.Errorf("fallback to email: ID = %q", got)
	}

	noIdentity := noID
	noIdentity.Email = ""
	if got := FromSession(noIdentity, now).ID; got != "token-"+shortHash("tok") {
		t.Errorf("fallback to token hash: ID = %q", got)
	}
}
