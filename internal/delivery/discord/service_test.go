package discord

import (
	"context"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/usecases/courseadmin"
)

// fakeGW records ApplicationCommandCreate, InteractionRespond, and
// InteractionResponseEdit calls.
type fakeGW struct {
	created  []*discordgo.ApplicationCommand
	responds []responded
	edits    []edited
}

type responded struct {
	interaction *discordgo.Interaction
	resp        *discordgo.InteractionResponse
}

type edited struct {
	interaction *discordgo.Interaction
	content     string
}

func (f *fakeGW) InteractionRespond(i *discordgo.Interaction, resp *discordgo.InteractionResponse, _ ...discordgo.RequestOption) error {
	f.responds = append(f.responds, responded{i, resp})
	return nil
}
func (f *fakeGW) InteractionResponseEdit(i *discordgo.Interaction, newresp *discordgo.WebhookEdit, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	c := ""
	if newresp != nil && newresp.Content != nil {
		c = *newresp.Content
	}
	f.edits = append(f.edits, edited{i, c})
	return nil, nil
}
func (f *fakeGW) ApplicationCommandCreate(_ string, _ string, cmd *discordgo.ApplicationCommand, _ ...discordgo.RequestOption) (*discordgo.ApplicationCommand, error) {
	f.created = append(f.created, cmd)
	return cmd, nil
}
func (f *fakeGW) User(_ string, _ ...discordgo.RequestOption) (*discordgo.User, error) {
	return &discordgo.User{ID: "botid"}, nil
}

type stubCourseAdmin struct {
	createCalled bool
	syncCalled   bool
	lastCourse   string
	lastLink     string
	createErr    error
	res          courseadmin.ProvisionResult
	syncN        int
}

func (s *stubCourseAdmin) Create(_ context.Context, courseId, link, _ string) (courseadmin.ProvisionResult, error) {
	s.createCalled = true
	s.lastCourse = courseId
	s.lastLink = link
	return s.res, s.createErr
}
func (s *stubCourseAdmin) Sync(_ context.Context, courseId, _ string) (int, error) {
	s.syncCalled = true
	s.lastCourse = courseId
	return s.syncN, nil
}

// newSvc builds a Service wired to a fake gateway + a stub course admin.
func newSvc(adminIDs []string, ca courseAdminAPI) (*Service, *fakeGW) {
	gw := &fakeGW{}
	return &Service{
		cfg:         &configs.Config{DiscordAdminIds: adminIDs},
		gw:          gw,
		admin:       &adminChecker{ids: adminIDs},
		courseAdmin: ca,
	}, gw
}

// cmdInteraction builds an application-command interaction with string options.
func cmdInteraction(userID, name string, opts ...*discordgo.ApplicationCommandInteractionDataOption) *discordgo.Interaction {
	i := &discordgo.Interaction{
		Type: discordgo.InteractionApplicationCommand,
		Data: discordgo.ApplicationCommandInteractionData{Name: name, Options: opts},
		Member: &discordgo.Member{User: &discordgo.User{ID: userID}},
	}
	return i
}

func optStr(name, val string) *discordgo.ApplicationCommandInteractionDataOption {
	return &discordgo.ApplicationCommandInteractionDataOption{
		Name: name, Value: val, Type: discordgo.ApplicationCommandOptionString,
	}
}

func TestAdminChecker(t *testing.T) {
	a := &adminChecker{ids: []string{"111", "222"}}
	if !a.isAdmin("111") || a.isAdmin("999") {
		t.Error("admin membership wrong")
	}
}

func TestStrOption(t *testing.T) {
	i := cmdInteraction("u", "create", optStr(optCourse, "CO2003-L01"), optStr(optCsvURL, "https://x/m.csv"))
	if got := strOption(i, optCourse); got != "CO2003-L01" {
		t.Errorf("course opt: %q", got)
	}
	if got := strOption(i, optCsvURL); got != "https://x/m.csv" {
		t.Errorf("csv opt: %q", got)
	}
	if got := strOption(i, "missing"); got != "" {
		t.Errorf("missing opt: %q", got)
	}
}

func TestCreate_AdminDenial(t *testing.T) {
	ca := &stubCourseAdmin{}
	s, gw := newSvc([]string{"admin1"}, ca)
	i := cmdInteraction("intruder", "create", optStr(optCourse, "X-L1"), optStr(optCsvURL, "https://x/m.csv"))

	s.handleCreate(i)

	if ca.createCalled {
		t.Error("use case should NOT be called for non-admin")
	}
	if len(gw.responds) != 1 {
		t.Fatalf("want 1 respond, got %d", len(gw.responds))
	}
	if !strings.Contains(gw.responds[0].resp.Data.Content, "không phải admin") {
		t.Errorf("denial message wrong: %q", gw.responds[0].resp.Data.Content)
	}
	if gw.responds[0].resp.Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
		t.Error("denial should be ephemeral")
	}
}

func TestCreate_AdminSuccess(t *testing.T) {
	ca := &stubCourseAdmin{res: courseadmin.ProvisionResult{
		CourseId: "CO2003-L01", Imported: 42, RoleID: "role:CO2003-L01", ChannelID: "chan:co2003-l01", Mapped: true,
	}}
	s, gw := newSvc([]string{"admin1"}, ca)
	i := cmdInteraction("admin1", "create", optStr(optCourse, "CO2003-L01"), optStr(optCsvURL, "https://x/m.csv"))

	s.handleCreate(i)

	if !ca.createCalled {
		t.Fatal("use case should be called for admin")
	}
	if ca.lastCourse != "CO2003-L01" || ca.lastLink != "https://x/m.csv" {
		t.Errorf("args forwarded wrong: course=%q link=%q", ca.lastCourse, ca.lastLink)
	}
	// First call is the deferred ACK (within Discord's 3s deadline), the result
	// comes back via an edit of the original response.
	if len(gw.responds) != 1 {
		t.Fatalf("want 1 deferred ACK, got %d", len(gw.responds))
	}
	if gw.responds[0].resp.Type != discordgo.InteractionResponseDeferredChannelMessageWithSource {
		t.Errorf("want deferred ACK, got type %v", gw.responds[0].resp.Type)
	}
	if len(gw.edits) != 1 {
		t.Fatalf("want 1 edit with the result, got %d", len(gw.edits))
	}
	content := gw.edits[0].content
	if !strings.Contains(content, "42") || !strings.Contains(content, "CO2003-L01") {
		t.Errorf("success edit missing details: %q", content)
	}
}

func TestCreate_ErrorShown(t *testing.T) {
	ca := &stubCourseAdmin{createErr: assertErr("boom")}
	s, gw := newSvc([]string{"admin1"}, ca)
	i := cmdInteraction("admin1", "create", optStr(optCourse, "X-L1"), optStr(optCsvURL, "bad"))

	s.handleCreate(i)

	if len(gw.responds) != 1 || gw.responds[0].resp.Type != discordgo.InteractionResponseDeferredChannelMessageWithSource {
		t.Fatal("want 1 deferred ACK first")
	}
	if len(gw.edits) != 1 {
		t.Fatal("want 1 edit surfacing the error")
	}
	if !strings.Contains(gw.edits[0].content, "boom") {
		t.Errorf("error not surfaced in edit: %q", gw.edits[0].content)
	}
}

func TestSync_AdminSuccess(t *testing.T) {
	ca := &stubCourseAdmin{syncN: 7}
	s, gw := newSvc([]string{"admin1"}, ca)
	i := cmdInteraction("admin1", "sync", optStr(optCourse, "CO2003-L01"))

	s.handleSync(i)

	if !ca.syncCalled || ca.lastCourse != "CO2003-L01" {
		t.Errorf("sync not forwarded: called=%v course=%q", ca.syncCalled, ca.lastCourse)
	}
	if len(gw.responds) != 1 || gw.responds[0].resp.Type != discordgo.InteractionResponseDeferredChannelMessageWithSource {
		t.Fatal("want 1 deferred ACK first")
	}
	if len(gw.edits) != 1 {
		t.Fatal("want 1 edit with the result")
	}
	if !strings.Contains(gw.edits[0].content, "7") {
		t.Errorf("sync count not shown in edit: %q", gw.edits[0].content)
	}
}

func TestRouteCommand_UnknownAndPlaceholder(t *testing.T) {
	s, gw := newSvc([]string{"a"}, &stubCourseAdmin{})
	s.routeCommand(nil, cmdInteraction("u", "nope"))
	s.routeCommand(nil, cmdInteraction("u", "mark"))
	if len(gw.responds) != 2 {
		t.Fatalf("want 2 responds, got %d", len(gw.responds))
	}
}

func TestApplicationCommands(t *testing.T) {
	cmds := applicationCommands()
	names := map[string]bool{}
	for _, c := range cmds {
		names[c.Name] = true
	}
	for _, want := range []string{cmdCreate, cmdSync, cmdBind, cmdMark, cmdProfile} {
		if !names[want] {
			t.Errorf("missing command %q", want)
		}
	}
}

func TestMessageBuilders(t *testing.T) {
	if ephemeralMsg("hi").Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
		t.Error("ephemeral flag missing")
	}
	if publicMsg("hi").Data.Flags&discordgo.MessageFlagsEphemeral != 0 {
		t.Error("public msg should not be ephemeral")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
