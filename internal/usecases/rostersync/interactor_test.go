package rostersync

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/student"
)

type fakeDownloader struct {
	records [][]string
	err     error
	calls   int
	lastURL string
}

func (f *fakeDownloader) DownloadCSV(url string) ([][]string, error) {
	f.calls++
	f.lastURL = url
	return f.records, f.err
}

// fakeRepo is an in-memory student.Repository. failOnMSSV forces an Upsert
// error for one MSSV to exercise per-row error isolation.
type fakeRepo struct {
	store      map[string]student.Model
	failOnMSSV string
	calls      int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{store: map[string]student.Model{}} }

func (r *fakeRepo) Upsert(m student.Model) error {
	r.calls++
	if m.MSSV == r.failOnMSSV {
		return errors.New("boom")
	}
	r.store[m.MSSV] = m
	return nil
}

func (r *fakeRepo) FindByEmail(email string) (student.Model, error) {
	return student.Model{}, student.ErrNotFound
}

func (r *fakeRepo) FindByMSSV(mssv string) (student.Model, error) {
	return student.Model{}, student.ErrNotFound
}

func cfg(url string) *configs.Config {
	return &configs.Config{RosterCsvUrl: url, RosterSyncInterval: time.Hour}
}

func TestSync_HappyPath_UpsertsAll(t *testing.T) {
	dl := &fakeDownloader{records: [][]string{
		{"MSSV", "Name", "Email"},
		{"2013307", "Nguyen Van A", "a@hcmut.edu.vn"},
		{"2013308", "Nguyen Van B", "b@hcmut.edu.vn"},
	}}
	repo := newFakeRepo()
	s := NewService(dl, repo, cfg("https://example.com/r.csv"))

	if err := s.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if dl.calls != 1 || dl.lastURL != "https://example.com/r.csv" {
		t.Errorf("download: calls=%d lastURL=%q", dl.calls, dl.lastURL)
	}
	if repo.calls != 2 {
		t.Errorf("upsert calls: want 2, got %d", repo.calls)
	}
	want := map[string]student.Model{
		"2013307": {MSSV: "2013307", Name: "Nguyen Van A", Email: "a@hcmut.edu.vn"},
		"2013308": {MSSV: "2013308", Name: "Nguyen Van B", Email: "b@hcmut.edu.vn"},
	}
	if !reflect.DeepEqual(repo.store, want) {
		t.Errorf("store: got %+v, want %+v", repo.store, want)
	}
}

func TestSync_DownloadError_Returned_NoUpserts(t *testing.T) {
	dlErr := errors.New("network down")
	dl := &fakeDownloader{err: dlErr}
	repo := newFakeRepo()
	s := NewService(dl, repo, cfg("https://example.com/r.csv"))

	if err := s.Sync(); err != dlErr {
		t.Errorf("want download error, got %v", err)
	}
	if repo.calls != 0 {
		t.Errorf("want 0 upserts on download error, got %d", repo.calls)
	}
}

func TestSync_EmptyURL_NoOp(t *testing.T) {
	dl := &fakeDownloader{}
	repo := newFakeRepo()
	s := NewService(dl, repo, cfg(""))

	if err := s.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if dl.calls != 0 || repo.calls != 0 {
		t.Errorf("want no work, got dl=%d repo=%d", dl.calls, repo.calls)
	}
}

func TestSync_PerRowError_DoesNotAbortBatch(t *testing.T) {
	dl := &fakeDownloader{records: [][]string{
		{"2013307", "A", "a@hcmut.edu.vn"},
		{"BAD", "B", "b@hcmut.edu.vn"},
		{"2013309", "C", "c@hcmut.edu.vn"},
	}}
	repo := newFakeRepo()
	repo.failOnMSSV = "BAD"
	s := NewService(dl, repo, cfg("https://example.com/r.csv"))

	if err := s.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, ok := repo.store["2013307"]; !ok {
		t.Error("want 2013307 upserted despite sibling failure")
	}
	if _, ok := repo.store["2013309"]; !ok {
		t.Error("want 2013309 upserted despite sibling failure")
	}
	if _, ok := repo.store["BAD"]; ok {
		t.Error("BAD should not be stored")
	}
}
