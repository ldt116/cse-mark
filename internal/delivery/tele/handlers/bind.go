package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"gopkg.in/telebot.v3"
	"thuanle/cse-mark/internal/delivery/tele/handlers/helpers"
	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/usecases/identity"
)

// bindStage tracks where a chat is in the bind conversation.
type bindStage int

const (
	stageIdle bindStage = iota
	stageAwaitEmail
	stageAwaitOTP
)

const platformTelegram = "telegram"

// dmOnlyMsg is the shared DM-only notice for identity features (/bind, bound
// /mark) — they never run in groups (issue #38).
const dmOnlyMsg = "Lệnh này chỉ dùng trong nhắn riêng (DM) với bot."

// isPrivate reports whether the update happened in a 1:1 chat with the bot.
// Identity features (/bind, bound /mark) are DM-only: in a group, keying by
// chat would turn the binding into a shared group credential (issue #38).
func isPrivate(c telebot.Context) bool {
	return c.Chat() != nil && c.Chat().Type == telebot.ChatPrivate
}

// Bind handles the conversational /bind flow: ask for email → BindStart → ask
// for OTP → BindVerify → confirm. State is held per sender (user) id in memory;
// a restart of the process clears in-flight binds (acceptable — the OTP record
// is also short-lived, and the user can re-run /bind).
type Bind struct {
	identity identityAPI

	mu     sync.Mutex
	stages map[int64]bindStage // key: telebot sender (user) id
	emails map[int64]string    // email pending verification per user
}

// identityAPI is the subset of identity.Service the tele bind handler uses.
type identityAPI interface {
	BindStart(ctx context.Context, platform, platformUserID, email string) error
	BindVerify(ctx context.Context, platform, platformUserID, otp string) (identity.BindResult, error)
	GetBinding(platform, platformUserID string) (binding.Model, error)
}

func NewBindHandler(identitySvc identityAPI) *Bind {
	return &Bind{
		identity: identitySvc,
		stages:   map[int64]bindStage{},
		emails:   map[int64]string{},
	}
}

// Start begins the bind flow. If already bound, it reports the existing MSSV.
func (h *Bind) Start(c telebot.Context) error {
	if !isPrivate(c) {
		return helpers.Send(c, dmOnlyMsg)
	}
	userID := c.Sender().ID
	if b, err := h.identity.GetBinding(platformTelegram, platformUserID(userID)); err == nil && b.Verified {
		return helpers.Sendf(c, "Bạn đã liên kết với MSSV %s.", b.MSSV)
	}
	h.setStage(userID, stageAwaitEmail)
	return helpers.Send(c, "Nhập email HCMUT của bạn (vd abc@hcmut.edu.vn):")
}

// OnText drives the conversation. It returns handled=true when the text was
// consumed by an in-flight bind, so the caller can skip the default mark path.
func (h *Bind) OnText(c telebot.Context) (handled bool, err error) {
	if !isPrivate(c) {
		return false, nil // group text is never bind input (issue #38)
	}
	userID := c.Sender().ID
	stage := h.stage(userID)
	if stage == stageIdle {
		return false, nil
	}
	text := strings.TrimSpace(c.Text())

	switch stage {
	case stageAwaitEmail:
		if err := h.identity.BindStart(context.Background(), platformTelegram, platformUserID(userID), text); err != nil {
			h.setStage(userID, stageIdle)
			return true, helpers.Send(c, teleBindStartMsg(err))
		}
		h.setEmail(userID, text)
		h.setStage(userID, stageAwaitOTP)
		return true, helpers.Send(c, "Đã gửi mã OTP tới "+text+". Nhập mã (hoặc /cancel để huỷ):")
	case stageAwaitOTP:
		res, err := h.identity.BindVerify(context.Background(), platformTelegram, platformUserID(userID), text)
		if err != nil {
			if resetOnVerifyErr(err) {
				h.setStage(userID, stageIdle)
			}
			return true, helpers.Send(c, teleBindVerifyMsg(err))
		}
		h.setStage(userID, stageIdle)
		return true, helpers.Sendf(c, "✅ Đã liên kết MSSV %s (%s).", res.MSSV, res.Name)
	}
	return false, nil
}

// Cancel aborts an in-flight bind. DM-only like the rest of the bind flow:
// a group /cancel must not touch sender state, and an anonymous group admin
// sends no `from` (Sender() is nil there) — so gate before using the sender.
func (h *Bind) Cancel(c telebot.Context) error {
	if !isPrivate(c) {
		return helpers.Send(c, dmOnlyMsg)
	}
	h.setStage(c.Sender().ID, stageIdle)
	return helpers.Send(c, "Đã huỷ liên kết.")
}

func (h *Bind) setStage(userID int64, s bindStage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stages[userID] = s
	if s == stageIdle {
		delete(h.emails, userID)
	}
}

func (h *Bind) stage(userID int64) bindStage {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stages[userID]
}

func (h *Bind) setEmail(userID int64, email string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emails[userID] = email
}

func platformUserID(userID int64) string { return fmt.Sprintf("%d", userID) }

// resetOnVerifyErr reports whether a verify error should clear the bind stage
// (because retrying the same OTP is pointless).
func resetOnVerifyErr(err error) bool {
	return errors.Is(err, identity.ErrOTPExpired) ||
		errors.Is(err, identity.ErrOTPMaxAttempts) ||
		errors.Is(err, identity.ErrNoPendingOTP)
}

func teleBindStartMsg(err error) string {
	switch {
	case errors.Is(err, identity.ErrEmailNotInRoster):
		return "Email chưa có trong danh sách sinh viên. Liên hệ admin nếu nhầm."
	case errors.Is(err, identity.ErrResendCooldown):
		return "Bạn vừa yêu cầu mã. Vui lòng đợi rồi dùng /bind lại."
	default:
		log.Warn().Err(err).Msg("tele bind start failed")
		return "Không thể gửi mã OTP lúc này. Thử lại sau."
	}
}

func teleBindVerifyMsg(err error) string {
	switch {
	case errors.Is(err, identity.ErrNoPendingOTP):
		return "Không có mã nào đang chờ. Dùng /bind để bắt đầu."
	case errors.Is(err, identity.ErrOTPExpired):
		return "Mã đã hết hạn. Dùng /bind để nhận mã mới."
	case errors.Is(err, identity.ErrOTPMaxAttempts):
		return "Nhập sai quá số lần. Dùng /bind để nhận mã mới (sau thời gian chờ)."
	case errors.Is(err, identity.ErrOTPIncorrect):
		return "Mã không đúng. Nhập lại hoặc /cancel."
	case errors.Is(err, identity.ErrMSSVAlreadyBound):
		return "MSSV này đã liên kết với tài khoản Telegram khác."
	default:
		log.Warn().Err(err).Msg("tele bind verify failed")
		return "Không thể xác thực lúc này. Thử lại sau."
	}
}
