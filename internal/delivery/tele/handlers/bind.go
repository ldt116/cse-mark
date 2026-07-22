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

// Bind handles the conversational /bind flow: ask for email → BindStart → ask
// for OTP → BindVerify → confirm. State is held per chat id in memory; a restart
// of the process clears in-flight binds (acceptable — the OTP record is also
// short-lived, and the user can re-run /bind).
type Bind struct {
	identity identityAPI

	mu     sync.Mutex
	stages map[int64]bindStage // key: telebot chat id
	emails map[int64]string    // email pending verification per chat
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
	chatID := c.Chat().ID
	if b, err := h.identity.GetBinding(platformTelegram, platformUserID(chatID)); err == nil && b.Verified {
		return helpers.Sendf(c, "Bạn đã liên kết với MSSV %s.", b.MSSV)
	}
	h.setStage(chatID, stageAwaitEmail)
	return helpers.Send(c, "Nhập email HCMUT của bạn (vd abc@hcmut.edu.vn):")
}

// OnText drives the conversation. It returns handled=true when the text was
// consumed by an in-flight bind, so the caller can skip the default mark path.
func (h *Bind) OnText(c telebot.Context) (handled bool, err error) {
	chatID := c.Chat().ID
	stage := h.stage(chatID)
	if stage == stageIdle {
		return false, nil
	}
	text := strings.TrimSpace(c.Text())

	switch stage {
	case stageAwaitEmail:
		if err := h.identity.BindStart(context.Background(), platformTelegram, platformUserID(chatID), text); err != nil {
			h.setStage(chatID, stageIdle)
			return true, helpers.Send(c, teleBindStartMsg(err))
		}
		h.setEmail(chatID, text)
		h.setStage(chatID, stageAwaitOTP)
		return true, helpers.Send(c, "Đã gửi mã OTP tới "+text+". Nhập mã (hoặc /cancel để huỷ):")
	case stageAwaitOTP:
		res, err := h.identity.BindVerify(context.Background(), platformTelegram, platformUserID(chatID), text)
		if err != nil {
			// keep the chat in OTP stage so they can retry, unless the OTP record
			// is gone/expired/maxed — those reset to idle.
			if resetOnVerifyErr(err) {
				h.setStage(chatID, stageIdle)
			}
			return true, helpers.Send(c, teleBindVerifyMsg(err))
		}
		h.setStage(chatID, stageIdle)
		return true, helpers.Sendf(c, "✅ Đã liên kết MSSV %s (%s).", res.MSSV, res.Name)
	}
	return false, nil
}

// Cancel aborts an in-flight bind.
func (h *Bind) Cancel(c telebot.Context) error {
	h.setStage(c.Chat().ID, stageIdle)
	return helpers.Send(c, "Đã huỷ liên kết.")
}

func (h *Bind) setStage(chatID int64, s bindStage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stages[chatID] = s
	if s == stageIdle {
		delete(h.emails, chatID)
	}
}

func (h *Bind) stage(chatID int64) bindStage {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stages[chatID]
}

func (h *Bind) setEmail(chatID int64, email string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emails[chatID] = email
}

func platformUserID(chatID int64) string { return fmt.Sprintf("%d", chatID) }

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
