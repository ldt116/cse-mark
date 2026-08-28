package handlers

import (
	"fmt"

	"gopkg.in/telebot.v3"
)

// fakeCtx satisfies telebot.Context by embedding the interface (nil) and
// overriding only the members the handlers under test call. Calling an
// un-overridden method panics — that is the desired loud failure.
type fakeCtx struct {
	telebot.Context

	chat   *telebot.Chat
	sender *telebot.User
	text   string
	args   []string
	sent   []string
}

func (f *fakeCtx) Chat() *telebot.Chat   { return f.chat }
func (f *fakeCtx) Sender() *telebot.User { return f.sender }
func (f *fakeCtx) Text() string          { return f.text }
func (f *fakeCtx) Args() []string        { return f.args }
func (f *fakeCtx) Send(msg interface{}, _ ...interface{}) error {
	f.sent = append(f.sent, fmt.Sprintf("%v", msg))
	return nil
}

// privateCtx builds a DM context. chatID differs from userID on purpose so
// tests can prove keying is by sender, not chat.
func privateCtx(chatID, userID int64) *fakeCtx {
	return &fakeCtx{
		chat:   &telebot.Chat{ID: chatID, Type: telebot.ChatPrivate},
		sender: &telebot.User{ID: userID},
	}
}

// groupCtx builds a group chat context (negative chat id, like real groups).
func groupCtx(userID int64) *fakeCtx {
	return &fakeCtx{
		chat:   &telebot.Chat{ID: -1001234, Type: telebot.ChatGroup},
		sender: &telebot.User{ID: userID},
	}
}
