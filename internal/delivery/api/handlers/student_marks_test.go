package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/jwks"
	"thuanle/cse-mark/internal/domain/mark"
	"thuanle/cse-mark/internal/delivery/api/middlewares"
	"thuanle/cse-mark/internal/usecases/assertion"
	"thuanle/cse-mark/internal/usecases/marksquery"
)

const (
	testIssuer   = "https://student-app.test"
	testAudience = "cse-mark"
)

// fakeJwks is a hand-rolled jwks.Repository; err, when set, makes every
// SigningKey call fail (JWKS endpoint down).
type fakeJwks struct {
	keys map[string]ed25519.PublicKey
	err  error
}

func (f *fakeJwks) SigningKey(kid string) (ed25519.PublicKey, error) {
	if f.err != nil {
		return nil, f.err
	}
	key, ok := f.keys[kid]
	if !ok {
		return nil, jwks.ErrUnknownKid
	}
	return key, nil
}

// fakeCourseRepo satisfies the full course.Repository interface and records
// whether FindCoursesUpdatedAfter ran.
type fakeCourseRepo struct {
	courses    []course.Model
	findCalled bool
}

func (f *fakeCourseRepo) FindCoursesUpdatedAfter(since time.Time) ([]course.Model, error) {
	f.findCalled = true
	return f.courses, nil
}
func (f *fakeCourseRepo) UpdateCourseRecordCount(string, int) error { return nil }
func (f *fakeCourseRepo) FindCoursesManagedByUser(string) ([]course.Model, error) {
	return nil, nil
}
func (f *fakeCourseRepo) FindCourseById(string) (course.Model, error) {
	return course.Model{}, course.ErrNotFound
}
func (f *fakeCourseRepo) UpdateCourseLink(string, string, int64, string) error { return nil }
func (f *fakeCourseRepo) RemoveCourse(string) error                           { return nil }

// fakeMarkRepo satisfies the full mark.Repository interface, records the last
// (courseId, studentId) pair it was asked for, and can fail on demand.
type fakeMarkRepo struct {
	marks        map[string]string // "courseId\x00studentId" -> JSON string
	err          error
	lastCourseId string
	lastStudent  string
}

func (f *fakeMarkRepo) GetMark(courseId string, studentId string) (string, error) {
	f.lastCourseId = courseId
	f.lastStudent = studentId
	if f.err != nil {
		return "", f.err
	}
	if m, ok := f.marks[courseId+"\x00"+studentId]; ok {
		return m, nil
	}
	return "", mark.ErrNotFound
}
func (f *fakeMarkRepo) RemoveMarksByCourseId(string) error { return nil }
func (f *fakeMarkRepo) AddCourseMarks(string, []map[string]string) error {
	return nil
}
func (f *fakeMarkRepo) RemoveCourseMarks(string) error { return nil }
func (f *fakeMarkRepo) ListStudentIds(string) ([]string, error) {
	return nil, nil
}

func newTestKeyPair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return priv, pub
}

// mintTestToken signs an EdDSA JWT the way the student app does (aud as a
// JSON array, kid in the header).
func mintTestToken(t *testing.T, priv ed25519.PrivateKey, kid, issuer, subject string, audience []string, expiresAt time.Time) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Audience:  jwt.ClaimStrings(audience),
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// newMarksEnv wires a real engine with the route registered exactly like
// service.go does: JWT middleware + StudentMarks.GetAll over the real
// assertion and marksquery services on fake repositories.
func newMarksEnv(t *testing.T, courseRepo *fakeCourseRepo, markRepo *fakeMarkRepo) (*gin.Engine, func(subject string) string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	priv, pub := newTestKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": pub}}
	jwtMw := middlewares.NewJwtMiddleware(assertion.NewService(repo, testIssuer, testAudience))
	studentMarks := NewStudentMarksHandler(marksquery.NewService(courseRepo, markRepo))

	engine := gin.New()
	engine.GET("/marks", jwtMw.Handle, studentMarks.GetAll)

	mint := func(subject string) string {
		return mintTestToken(t, priv, "key-a", testIssuer, subject, []string{testAudience}, time.Now().Add(5*time.Minute))
	}
	return engine, mint
}

func serveMarks(engine *gin.Engine, token, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/marks"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestStudentMarksGetAll_Valid(t *testing.T) {
	courseRepo := &fakeCourseRepo{courses: []course.Model{
		{Id: "c1", UpdatedAt: time.Now().Unix()},
		{Id: "c2", UpdatedAt: time.Now().Unix()},
	}}
	markRepo := &fakeMarkRepo{marks: map[string]string{
		"c1\x002111111": `{"z_total":10,"a_mid":4}`,
	}}
	engine, mint := newMarksEnv(t, courseRepo, markRepo)

	w := serveMarks(engine, mint("2111111"), "")

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
	// Unmarshal to check the shape; c2 has no mark document -> skipped.
	var got []struct {
		CourseId string          `json:"courseId"`
		Marks    json.RawMessage `json:"marks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not the expected JSON array: %v (body=%s)", err, w.Body.String())
	}
	if len(got) != 1 {
		t.Fatalf("len(body) = %d, want 1 (c2 skipped)", len(got))
	}
	if got[0].CourseId != "c1" {
		t.Errorf("courseId = %q, want %q", got[0].CourseId, "c1")
	}
	if string(got[0].Marks) != `{"z_total":10,"a_mid":4}` {
		t.Errorf("marks = %s, want verbatim %s", string(got[0].Marks), `{"z_total":10,"a_mid":4}`)
	}
}

func TestStudentMarksGetAll_CourseFilterPassedThrough(t *testing.T) {
	courseRepo := &fakeCourseRepo{courses: []course.Model{{Id: "c1"}, {Id: "c3"}}}
	markRepo := &fakeMarkRepo{marks: map[string]string{
		"c1\x002111111": `{"z_total":10}`,
	}}
	engine, mint := newMarksEnv(t, courseRepo, markRepo)

	w := serveMarks(engine, mint("2111111"), "?course_id=c1")

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if courseRepo.findCalled {
		t.Error("FindCoursesUpdatedAfter called for a course_id query, want the filter branch")
	}
	if markRepo.lastCourseId != "c1" {
		t.Errorf("mark repo received courseId %q, want %q", markRepo.lastCourseId, "c1")
	}
}

// TestStudentMarksGetAll_IgnoresStudentParam is the security tooth of this
// handler: a ?student= query parameter must be ignored — the student id comes
// from the verified token subject only.
func TestStudentMarksGetAll_IgnoresStudentParam(t *testing.T) {
	courseRepo := &fakeCourseRepo{courses: []course.Model{{Id: "c1"}}}
	markRepo := &fakeMarkRepo{marks: map[string]string{
		"c1\x002111111": `{"z_total":10}`,
	}}
	engine, mint := newMarksEnv(t, courseRepo, markRepo)

	w := serveMarks(engine, mint("2111111"), "?student=2119999")

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if markRepo.lastStudent != "2111111" {
		t.Errorf("mark repo received studentId %q, want token subject %q (?student must be ignored)",
			markRepo.lastStudent, "2111111")
	}
	var got []map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not the expected JSON array: %v (body=%s)", err, w.Body.String())
	}
	if len(got) != 1 {
		t.Fatalf("len(body) = %d, want 1 (marks of 2111111, not of 2119999)", len(got))
	}
}

func TestStudentMarksGetAll_NoMarks(t *testing.T) {
	courseRepo := &fakeCourseRepo{courses: []course.Model{{Id: "c1"}}}
	markRepo := &fakeMarkRepo{marks: map[string]string{}}
	engine, mint := newMarksEnv(t, courseRepo, markRepo)

	w := serveMarks(engine, mint("2111111"), "")

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("body = %s, want []", w.Body.String())
	}
}

func TestStudentMarksGetAll_RepoError(t *testing.T) {
	courseRepo := &fakeCourseRepo{courses: []course.Model{{Id: "c1"}}}
	markRepo := &fakeMarkRepo{err: errors.New("connection lost")}
	engine, mint := newMarksEnv(t, courseRepo, markRepo)

	w := serveMarks(engine, mint("2111111"), "")

	if w.Code != 500 {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"error":"internal_error"}` {
		t.Errorf("body = %s, want {\"error\":\"internal_error\"}", got)
	}
}

// TestStudentMarksGetAll_RejectsBadToken pins the auth boundary of the route:
// without a valid assertion the handler must not run at all.
func TestStudentMarksGetAll_RejectsBadToken(t *testing.T) {
	courseRepo := &fakeCourseRepo{courses: []course.Model{{Id: "c1"}}}
	markRepo := &fakeMarkRepo{marks: map[string]string{}}
	engine, _ := newMarksEnv(t, courseRepo, markRepo)

	req := httptest.NewRequest(http.MethodGet, "/marks", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if markRepo.lastStudent != "" {
		t.Errorf("mark repo was queried with studentId %q, want no query at all", markRepo.lastStudent)
	}
}
