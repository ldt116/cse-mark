package rostersync

import (
	"errors"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/downloader"
	"thuanle/cse-mark/internal/domain/student"
)

// Service periodically downloads the roster CSV and upserts students, providing
// the stable email -> MSSV trust source used by the bind flow. It is fully
// independent of mark sync: it runs as its own goroutine and its failures never
// affect the mark sync loop.
type Service struct {
	downloader   downloader.Repository
	studentRepo  student.Repository
	csvUrl       string
	syncInterval time.Duration
}

// NewService wires the roster sync usecase. Reading RosterCsvUrl /
// RosterSyncInterval from config matches the existing provider style
// (e.g. course.NewRules, mongo.NewStudentRepo).
func NewService(downloader downloader.Repository, studentRepo student.Repository, config *configs.Config) *Service {
	return &Service{
		downloader:   downloader,
		studentRepo:  studentRepo,
		csvUrl:       config.RosterCsvUrl,
		syncInterval: config.RosterSyncInterval,
	}
}

// Sync runs one roster sync cycle: download, parse, upsert. It returns the
// download error (if any) so the caller can log it. Per-student upsert errors
// (e.g. a duplicate email hitting the unique index) are logged and skipped so a
// single bad row never aborts the batch. When csvUrl is empty it is a no-op.
func (s *Service) Sync() error {
	if s.csvUrl == "" {
		log.Warn().Msg("ROSTER_CSV_URL not set; skipping roster sync")
		return nil
	}

	log.Info().Str("host", hostOf(s.csvUrl)).Msg("Syncing roster")
	records, err := s.downloader.DownloadCSV(s.csvUrl)
	if err != nil {
		// net/http wraps transport errors as *url.Error, whose Error() embeds
		// the full ROSTER_CSV_URL (including the secret token in the path).
		// Redact it so Sync's caller never logs the secret.
		return redactURLErr(err)
	}

	students := parseRoster(records)

	upserted, failed := 0, 0
	for _, m := range students {
		if err := s.studentRepo.Upsert(m); err != nil {
			log.Warn().Err(err).Str("mssv", m.MSSV).Msg("Upsert student failed; skipping")
			failed++
			continue
		}
		upserted++
	}

	log.Info().
		Int("downloaded", len(records)).
		Int("upserted", upserted).
		Int("skipped", len(records)-len(students)).
		Int("failed", failed).
		Msg("Roster sync complete")
	return nil
}

// Run runs Sync on RosterSyncInterval forever. It is independent of mark sync
// (launched as its own goroutine by cmd/fetcher) and tolerates per-cycle errors
// by logging them and continuing. When csvUrl is empty, roster sync is disabled
// and Run returns immediately.
func (s *Service) Run() {
	if s.csvUrl == "" {
		log.Warn().Msg("ROSTER_CSV_URL not set; roster sync disabled")
		return
	}
	interval := s.syncInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	for {
		if err := s.Sync(); err != nil {
			log.Error().Err(err).Msg("Roster sync cycle failed")
		}
		log.Info().Str("interval", interval.String()).Msg("Roster sync sleeping")
		time.Sleep(interval)
	}
}

// hostOf returns the URL's host for logging. ROSTER_CSV_URL is a SOPS secret
// whose access token lives in the path/query, so only the host is logged —
// never the full URL. It returns "<invalid>" (not the raw URL) on a parse
// failure so a malformed secret is never written to logs either.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "<invalid>"
	}
	return u.Host
}

// redactURLErr strips the secret URL from a download error before it can be
// logged. net/http wraps transport failures as *url.Error whose string form is
// `Get "<full URL>": <reason>` — that string would leak ROSTER_CSV_URL (token
// included) into logs. We unwrap the underlying reason and drop the URL, so the
// caller only sees the cause (e.g. "context deadline exceeded"), not the URL.
func redactURLErr(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Err != nil {
			return urlErr.Err
		}
		return errors.New(urlErr.Op + ": <redacted url>")
	}
	return err
}

