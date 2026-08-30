package middlewares

import (
	tele "gopkg.in/telebot.v3"
)

// Whitelist chỉ cho phép sender id trong danh sách. Khác middleware upstream
// (telebot v3 middleware.Whitelist), update KHÔNG có sender (anonymous group
// admin chỉ có sender_chat) bị bỏ qua im lặng thay vì panic nil deref.
func Whitelist(chats ...int64) tele.MiddlewareFunc {
	set := make(map[int64]struct{}, len(chats))
	for _, id := range chats {
		set[id] = struct{}{}
	}
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			s := c.Sender()
			if s == nil {
				return nil
			}
			if _, ok := set[s.ID]; !ok {
				return nil
			}
			return next(c)
		}
	}
}
