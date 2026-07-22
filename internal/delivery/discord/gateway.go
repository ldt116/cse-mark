package discord

import (
	"context"

	"github.com/bwmarrin/discordgo"
)

// interactionGateway is the subset of *discordgo.Session the handlers use to
// read and respond to interactions. Defining it as an interface lets the
// handlers be unit-tested with a fake session that records the responses it was
// asked to send. Production binds the real *discordgo.Session.
type interactionGateway interface {
	InteractionRespond(i *discordgo.Interaction, resp *discordgo.InteractionResponse, opts ...discordgo.RequestOption) error
	InteractionResponseEdit(i *discordgo.Interaction, newresp *discordgo.WebhookEdit, opts ...discordgo.RequestOption) (*discordgo.Message, error)
	ApplicationCommandCreate(appID, guildID string, cmd *discordgo.ApplicationCommand, opts ...discordgo.RequestOption) (*discordgo.ApplicationCommand, error)
	User(userID string, opts ...discordgo.RequestOption) (*discordgo.User, error)
}

// ephemeralMsg builds a channel-message interaction response that only the
// caller sees. Used for all sensitive output (marks, bind details).
func ephemeralMsg(content string) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}
}

// publicMsg builds a visible-to-all interaction response (e.g. admin confirmations).
func publicMsg(content string) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content},
	}
}

// deferredMsg builds a DEFERRED response ("Bot is thinking..."). Discord allows
// up to 15 minutes to follow up after a deferred ACK, so long-running admin
// commands (import/provision) ACK within the 3s deadline and then edit the
// original response with their result.
func deferredMsg(content string) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content},
	}
}

// editInteraction updates the original deferred response with the given text. It
// is the follow-up half of the defer-then-edit pattern used by /create and /sync.
func editInteraction(gw interactionGateway, i *discordgo.Interaction, content string) error {
	_, err := gw.InteractionResponseEdit(i, &discordgo.WebhookEdit{Content: &content})
	return err
}

// withCtx is a no-op helper kept for symmetry with future context-aware calls.
func withCtx(_ context.Context) {}
