package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/domain/student"
	"thuanle/cse-mark/internal/usecases/identity"
)

const platformDiscord = "discord"

// --- /bind ---

// handleBind opens the email-input modal. The simplest coherent thread: a single
// modal collects the HCMUT email, submit triggers BindStart; if an OTP is issued
// the response message carries a Verify button that opens the OTP modal. This
// keeps the flow to two modals (email, then OTP) with no free-text conversation,
// matching the "luồng đơn giản nhất" decision.
func (s *Service) handleBind(i *discordgo.Interaction) {
	if s.identity == nil {
		_ = s.gw.InteractionRespond(i, ephemeralMsg("Chức năng liên kết chưa được bật."))
		return
	}
	// Already-bound users get a quick note rather than re-entering everything.
	if b, err := s.identity.GetBinding(platformDiscord, caller(i)); err == nil && b.Verified {
		_ = s.gw.InteractionRespond(i, ephemeralMsg(fmt.Sprintf("Bạn đã liên kết với MSSV %s. Dùng /profile để xem chi tiết.", b.MSSV)))
		return
	}
	_ = s.gw.InteractionRespond(i, emailModalResponse())
}

// emailModalResponse builds the modal that collects the HCMUT email.
func emailModalResponse() *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: cidBindEmailModal,
			Title:    "Liên kết tài khoản",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					&discordgo.TextInput{
						CustomID:    cidBindEmailModal + ":email",
						Label:       "Email HCMUT",
						Placeholder: "abc@hcmut.edu.vn",
						Style:       discordgo.TextInputShort,
						Required:    true,
					},
				}},
			},
		},
	}
}

// otpModalResponse builds the modal that collects the OTP, keyed to the email so
// the submit handler can re-resolve the verification by platform user id.
func otpModalResponse(email string) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: cidBindOtpModal + email,
			Title:    "Nhập mã OTP",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					&discordgo.TextInput{
						CustomID: cidBindOtpModal + "code",
						Label:    "Mã OTP (6 chữ số)",
						Style:    discordgo.TextInputShort,
						Required: true,
					},
				}},
			},
		},
	}
}

// routeModal (override of the stub) now drives the bind modal flow.
func (s *Service) routeModalImpl(_ *discordgo.Session, i *discordgo.Interaction) {
	data := i.ModalSubmitData()
	switch {
	case data.CustomID == cidBindEmailModal:
		s.onEmailSubmit(i, data)
	case strings.HasPrefix(data.CustomID, cidBindOtpModal):
		s.onOtpSubmit(i, data)
	default:
		_ = s.gw.InteractionRespond(i, ephemeralMsg("Không hỗ trợ modal này."))
	}
}

// onEmailSubmit runs BindStart with the submitted email. On success it replies
// with a Verify button opening the OTP modal; on a known error it explains.
func (s *Service) onEmailSubmit(i *discordgo.Interaction, data discordgo.ModalSubmitInteractionData) {
	email := firstInput(data, cidBindEmailModal+":email")
	err := s.identity.BindStart(context.Background(), platformDiscord, caller(i), email)
	if err != nil {
		_ = s.gw.InteractionRespond(i, ephemeralMsg(bindStartMsg(err)))
		return
	}
	_ = s.gw.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: "Đã gửi mã OTP tới " + email + ". Bấm **Xác thực** để nhập mã.",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					&discordgo.Button{
						Label:    "Xác thực",
						Style:    discordgo.PrimaryButton,
						CustomID: cidBindVerifyBtn,
					},
				}},
			},
		},
	})
}

// onOtpSubmit runs BindVerify and reports the result.
func (s *Service) onOtpSubmit(i *discordgo.Interaction, data discordgo.ModalSubmitInteractionData) {
	otp := firstInput(data, cidBindOtpModal+"code")
	res, err := s.identity.BindVerify(context.Background(), platformDiscord, caller(i), otp)
	if err != nil {
		_ = s.gw.InteractionRespond(i, ephemeralMsg(bindVerifyMsg(err)))
		return
	}
	_ = s.gw.InteractionRespond(i, ephemeralMsg(fmt.Sprintf("✅ Đã liên kết MSSV %s (%s). Role các lớp đang học sẽ được cấp ở chu kỳ đồng bộ kế tiếp.", res.MSSV, res.Name)))
}

// routeComponentImpl handles the Verify button: it opens the OTP modal.
func (s *Service) routeComponentImpl(_ *discordgo.Session, i *discordgo.Interaction) {
	if i.MessageComponentData().CustomID == cidBindVerifyBtn {
		_ = s.gw.InteractionRespond(i, otpModalResponse(""))
		return
	}
	_ = s.gw.InteractionRespond(i, ephemeralMsg("Tương tác chưa được kết nối."))
}

// bindStartMsg maps identity errors to a localized message for the email step.
func bindStartMsg(err error) string {
	switch {
	case errors.Is(err, identity.ErrEmailNotInRoster):
		return "Email chưa có trong danh sách sinh viên (roster). Hỏi admin nếu bạn nghĩ đây là nhầm lẫn."
	case errors.Is(err, identity.ErrResendCooldown):
		return "Bạn vừa yêu cầu mã. Vui lòng đợi một lát rồi dùng lại /bind."
	default:
		log.Warn().Err(err).Msg("bind start failed")
		return "Không thể gửi mã OTP lúc này. Thử lại sau."
	}
}

// bindVerifyMsg maps identity errors to a localized message for the OTP step.
func bindVerifyMsg(err error) string {
	switch {
	case errors.Is(err, identity.ErrNoPendingOTP):
		return "Không có yêu cầu liên kết nào đang chờ. Dùng /bind để bắt đầu."
	case errors.Is(err, identity.ErrOTPExpired):
		return "Mã đã hết hạn. Dùng /bind để nhận mã mới."
	case errors.Is(err, identity.ErrOTPMaxAttempts):
		return "Bạn đã nhập sai quá số lần cho phép. Dùng /bind để nhận mã mới (sau thời gian chờ)."
	case errors.Is(err, identity.ErrOTPIncorrect):
		return "Mã không đúng. Vui lòng nhập lại (bấm Xác thực)."
	case errors.Is(err, identity.ErrMSSVAlreadyBound):
		return "MSSV này đã được liên kết với tài khoản khác trên Discord."
	default:
		log.Warn().Err(err).Msg("bind verify failed")
		return "Không thể xác thực lúc này. Thử lại sau."
	}
}

// firstInput reads the first text-input value from a submitted modal by custom id.
func firstInput(data discordgo.ModalSubmitInteractionData, customID string) string {
	for _, row := range data.Components {
		ar, ok := row.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, c := range ar.Components {
			if ti, ok := c.(*discordgo.TextInput); ok && ti.CustomID == customID {
				return strings.TrimSpace(ti.Value)
			}
		}
	}
	return ""
}

// --- /profile ---

// handleProfile shows MSSV/name/email and enrolled classes for the bound user.
func (s *Service) handleProfile(i *discordgo.Interaction) {
	if s.identity == nil || s.studentRepo == nil {
		_ = s.gw.InteractionRespond(i, ephemeralMsg("Chức năng hồ sơ chưa được bật."))
		return
	}
	b, err := s.identity.GetBinding(platformDiscord, caller(i))
	if err != nil {
		_ = s.gw.InteractionRespond(i, ephemeralMsg(notBoundMsg))
		return
	}
	stu, err := s.studentRepo.FindByMSSV(b.MSSV)
	if err != nil {
		stu = student.Model{MSSV: b.MSSV}
	}
	classes := s.enrolledClasses(b.MSSV)
	_ = s.gw.InteractionRespond(i, ephemeralMsg(formatProfile(stu, classes)))
}

func formatProfile(stu student.Model, classes []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Hồ sơ\nMSSV: %s\nHọ tên: %s\nEmail: %s\n", stu.MSSV, stu.Name, stu.Email)
	if len(classes) == 0 {
		b.WriteString("Lớp đang học: (chưa có)")
	} else {
		b.WriteString("Lớp đang học: " + strings.Join(classes, ", "))
	}
	return b.String()
}

// enrolledClasses returns course ids (active) in which the MSSV has a mark record.
func (s *Service) enrolledClasses(mssv string) []string {
	// We only know courses via the course repo; cross-check each active course's
	// mark collection for the MSSV. For large course sets this is N lookups; an
	// enrollment index would be better, but the mark cache is the source of truth
	// (SRS §13 Enrollment) and course counts are modest for a department LMS.
	var out []string
	// courseRepo doesn't expose "all"; use FindCoursesUpdatedAfter(0) = all with
	// an updatedAt >= epoch, which the repo implements via FindCoursesUpdatedAfter.
	courses, err := s.courseRepo.FindCoursesUpdatedAfter(epochZero())
	if err != nil {
		return out
	}
	for _, c := range courses {
		if _, err := s.markRepo.GetMark(c.Id, mssv); err == nil {
			out = append(out, c.Id)
		}
	}
	return out
}

// --- /mark ---

// handleMark returns the bound student's marks. With a course option: that
// course's marks; without: every enrolled course.
func (s *Service) handleMark(i *discordgo.Interaction) {
	if s.identity == nil || s.markRepo == nil {
		_ = s.gw.InteractionRespond(i, ephemeralMsg("Chức năng tra điểm chưa được bật."))
		return
	}
	b, err := s.identity.GetBinding(platformDiscord, caller(i))
	if err != nil {
		_ = s.gw.InteractionRespond(i, ephemeralMsg(notBoundMsg))
		return
	}
	courseId := strOption(i, optCourse)
	if courseId != "" {
		m, err := s.markRepo.GetMark(courseId, b.MSSV)
		if err != nil {
			_ = s.gw.InteractionRespond(i, ephemeralMsg(fmt.Sprintf("Chưa có điểm cho %s.", courseId)))
			return
		}
		_ = s.gw.InteractionRespond(i, ephemeralMsg(fmt.Sprintf("Điểm %s:\n%s", courseId, collapseJSON(m))))
		return
	}
	// all enrolled courses
	var b2 strings.Builder
	any := false
	courses, err := s.courseRepo.FindCoursesUpdatedAfter(epochZero())
	if err == nil {
		for _, c := range courses {
			m, err := s.markRepo.GetMark(c.Id, b.MSSV)
			if err != nil {
				continue
			}
			any = true
			fmt.Fprintf(&b2, "%s:\n%s\n", c.Id, collapseJSON(m))
		}
	}
	if !any {
		_ = s.gw.InteractionRespond(i, ephemeralMsg("Chưa có điểm nào."))
		return
	}
	_ = s.gw.InteractionRespond(i, ephemeralMsg(b2.String()))
}

// notBoundMsg is the message shown when a student command is used before binding.
const notBoundMsg = "Chưa xác thực. Dùng /bind để liên kết tài khoản với MSSV."

// epochZero returns the Unix epoch, used as the "all courses" lower bound for
// FindCoursesUpdatedAfter.
func epochZero() time.Time { return time.Unix(0, 0) }

// collapseJSON turns the indented JSON mark blob into a compact line-per-field
// block for readability inside Discord. It is defensive: on any parse error it
// returns the raw string.
func collapseJSON(raw string) string {
	// mark repo returns MarshalIndent with two spaces; trim the indent to one
	// space per level and strip braces for a cleaner paste.
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimSpace(ln)
	}
	out := strings.Join(lines, "\n")
	out = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(out, "\n"), "\n"))
	if out == "" {
		return raw
	}
	return out
}
