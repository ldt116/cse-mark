package main

import (
	emailport "thuanle/cse-mark/internal/domain/email"
	"thuanle/cse-mark/internal/usecases/identity"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/mark"
	"thuanle/cse-mark/internal/delivery/tele/handlers"
	"thuanle/cse-mark/internal/infra/email"
)

// ProvideSender selects the OTP delivery sender from config: SmtpSender when
// SMTP is configured, FailingSender (bind fails closed) otherwise, LogSender
// only via OTP_SENDER=log dev opt-in. Telegram needs email solely to deliver
// OTPs for /bind; this keeps production honest (no log-and-claim).
func ProvideSender(config *configs.Config) emailport.Sender {
	return email.NewSenderFromConfig(config)
}

// ProvideGuestHandler builds the guest handler with identity injected so /mark
// resolves MSSV from the binding (v2), plus the course repository so /mark
// without a course arg summarizes every enrolled course. identity is a concrete
// *identity.Service, which satisfies the identityLookup interface via GetBinding.
func ProvideGuestHandler(rules *course.Rules, markRepo mark.Repository, courseRepo course.Repository, ident *identity.Service) *handlers.Guest {
	return handlers.NewGuestHandler(rules, markRepo,
		handlers.WithGuestIdentity(ident),
		handlers.WithGuestCourseLister(courseRepo),
	)
}

// ProvideBindHandler wraps handlers.NewBindHandler for wire: the handler's
// identityAPI parameter is an unexported interface, which wire cannot bind
// from this package. *identity.Service satisfies it.
func ProvideBindHandler(ident *identity.Service) *handlers.Bind {
	return handlers.NewBindHandler(ident)
}
