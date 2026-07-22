package main

import (
	emailport "thuanle/cse-mark/internal/domain/email"
	"thuanle/cse-mark/internal/infra/email"
	"thuanle/cse-mark/internal/usecases/identity"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/mark"
	"thuanle/cse-mark/internal/delivery/tele/handlers"
)

// ProvideLogSender returns a no-op email sender. Swap for SMTP once SMTP_* are
// configured. Telegram needs email only to deliver OTPs for /bind.
func ProvideLogSender() emailport.Sender {
	return email.NewLogSender()
}

// ProvideGuestHandler builds the guest handler with identity injected so /mark
// resolves MSSV from the binding (v2). identity is a concrete *identity.Service,
// which satisfies the identityLookup interface via GetBinding.
func ProvideGuestHandler(rules *course.Rules, markRepo mark.Repository, ident *identity.Service) *handlers.Guest {
	return handlers.NewGuestHandler(rules, markRepo, handlers.WithGuestIdentity(ident))
}
