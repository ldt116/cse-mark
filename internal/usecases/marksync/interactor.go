package marksync

import (
	"errors"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/downloader"
	"thuanle/cse-mark/internal/usecases/coursequery"
	"thuanle/cse-mark/internal/usecases/markimport"
)

type Service struct {
	courseQueryService *coursequery.ActiveCourseService
	courseRepo         course.Repository
	markImportService  *markimport.Service

	fetchingInterval   time.Duration
	slowPollInterval   time.Duration // re-probe delay after config/permanent errors
	transientThreshold int           // consecutive transient failures before "unhealthy"

	consecutiveFailures map[string]int
	nextAttemptAt       map[string]time.Time
	lastTokenErrorLog   time.Time
}

const defaultTransientThreshold = 6

func NewService(courseQueryService *coursequery.ActiveCourseService,
	courseRepo course.Repository, markImportService *markimport.Service) *Service {
	return &Service{
		courseQueryService: courseQueryService,
		courseRepo:         courseRepo,
		markImportService:  markImportService,

		fetchingInterval:   time.Minute,
		slowPollInterval:   time.Hour,
		transientThreshold: defaultTransientThreshold,

		consecutiveFailures: map[string]int{},
		nextAttemptAt:       map[string]time.Time{},
	}
}

// fetchClass buckets a FetchMarkLinkIntoCourse error by the recovery action it
// demands (hcmut-util spec 29 feed contract).
type fetchClass int

const (
	classSuccess fetchClass = iota
	classTransient
	classConfigToken
	classPermanentMon
	classPermanentGrant
)

func classifyFetchErr(err error) fetchClass {
	if err == nil {
		return classSuccess
	}
	var fe *downloader.FeedError
	if !errors.As(err, &fe) {
		return classTransient // network, parse, mongo — none is feed-policy
	}
	switch {
	case fe.Status == http.StatusUnauthorized && fe.Code == "service_token_invalid":
		return classConfigToken
	case fe.Status == http.StatusForbidden || fe.Status == http.StatusNotFound:
		return classPermanentMon
	case fe.Status == http.StatusGone:
		return classPermanentGrant
	default:
		return classTransient // 429, 5xx, unknown codes
	}
}

// syncCourse fetches one course's feed and applies the error-class transition:
// config-token/permanent-mon → slow-poll hourly, permanent-grant → inactive,
// transient → count towards the unhealthy threshold, success → heal stale.
func (s *Service) syncCourse(c course.Model, now time.Time) {
	if s.nextAttemptAt[c.Id].After(now) {
		return
	}

	_, err := s.markImportService.FetchMarkLinkIntoCourse(c.Id, c.Link)

	switch classifyFetchErr(err) {
	case classSuccess:
		s.consecutiveFailures[c.Id] = 0
		delete(s.nextAttemptAt, c.Id)
		if c.EffectiveStatus() != course.StatusActive {
			// auto-heal: the hourly stale probe succeeded — resume normal polling
			if err := s.courseRepo.SetCourseStatus(c.Id, course.StatusActive); err != nil {
				log.Error().Err(err).Str("courseId", c.Id).Msg("Failed to heal course status")
			}
		}
	case classTransient:
		s.consecutiveFailures[c.Id]++
		if s.consecutiveFailures[c.Id] == s.transientThreshold {
			log.Warn().Str("courseId", c.Id).
				Int("consecutive", s.consecutiveFailures[c.Id]).
				Msg("feed unhealthy")
		} else {
			log.Info().Str("courseId", c.Id).Err(err).Msg("mark fetch failed (transient)")
		}
	case classConfigToken:
		s.nextAttemptAt[c.Id] = now.Add(s.slowPollInterval)
		if now.Sub(s.lastTokenErrorLog) >= s.slowPollInterval {
			log.Error().Str("courseId", c.Id).
				Msg("service token rejected (service_token_invalid) — check GV_PROXY_TOKEN; polling hourly")
			s.lastTokenErrorLog = now
		}
	case classPermanentMon:
		s.nextAttemptAt[c.Id] = now.Add(s.slowPollInterval)
		if err := s.courseRepo.SetCourseStatus(c.Id, course.StatusStale); err != nil {
			log.Error().Err(err).Str("courseId", c.Id).Msg("Failed to mark course stale")
		}
		log.Warn().Str("courseId", c.Id).Err(err).
			Msg("course stale — keeping last good marks, probing hourly")
	case classPermanentGrant:
		if err := s.courseRepo.SetCourseStatus(c.Id, course.StatusInactive); err != nil {
			log.Error().Err(err).Str("courseId", c.Id).Msg("Failed to mark course inactive")
		}
		log.Warn().Str("courseId", c.Id).
			Msg("grant revoked — course inactive, marks frozen (admin decides teardown)")
	}
}

func (s *Service) fetchNewMarks() {
	log.Info().Msg("Fetching new marks for all classes")

	courses, err := s.courseQueryService.ListActiveCourses()

	if err != nil {
		log.Error().Err(err).Msg("Fetching active courses error")
		return
	}

	now := time.Now()
	for _, course := range courses {
		s.syncCourse(course, now)
		time.Sleep(s.fetchingInterval)
	}
}

func (s *Service) Run() {
	for {
		s.fetchNewMarks()
		log.Info().Msg("Sleeping for 10 minutes...")
		time.Sleep(10 * time.Minute)
	}
}
