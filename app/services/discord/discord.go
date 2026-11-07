package discord

import (
	"errors"
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"

	"goravel/app/facades"
)

// ErrNotConfigured is returned when the bot token is empty: Discord is turned
// off, so no message can be sent and no thread can be created or deleted.
var ErrNotConfigured = errors.New("discord: DISCORD_BOT_TOKEN is not configured")

var (
	discordSession *discordgo.Session
	once           sync.Once
)

type Discord interface {
	Close()
	CreateThread(channelID string, thread Thread) (string, error)
	DeleteThread(threadID string) error
	SendMessage(channelID string, message string) error
}

type Thread struct {
	Title   string
	Content string
}

type DiscordImpl struct {
	session *discordgo.Session
}

func NewDiscord() (*DiscordImpl, error) {
	// Without a token, discordgo.New("Bot ") would build a session that fails
	// Open() against the gateway on every boot, so treat it as disabled.
	if facades.Config().GetString("discord.bot.token") == "" {
		if logger := facades.Log(); logger != nil {
			logger.Info("discord: DISCORD_BOT_TOKEN is empty, Discord is disabled")
		}

		return &DiscordImpl{}, nil
	}

	var err error
	once.Do(func() {
		discordSession, err = discordgo.New("Bot " + facades.Config().GetString("discord.bot.token"))
		if err != nil {
			return
		}

		if err = discordSession.Open(); err != nil {
			return
		}
	})
	if err != nil {
		return nil, err
	}

	return &DiscordImpl{
		session: discordSession,
	}, nil
}

func (r *DiscordImpl) Close() {
	_ = r.session.Close()
}

// CloseDiscord closes the process-wide Discord session if one was opened. It
// is a no-op when no session exists.
func CloseDiscord() {
	if discordSession != nil {
		_ = discordSession.Close()
	}
}

func (r *DiscordImpl) CreateThread(channelID string, thread Thread) (string, error) {
	if r.session == nil {
		return "", ErrNotConfigured
	}

	createdThread, err := r.session.ThreadStartComplex(channelID, &discordgo.ThreadStart{
		Name: thread.Title,
		Type: discordgo.ChannelTypeGuildPublicThread,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create thread for pull request: %w", err)
	}

	if err := r.SendMessage(createdThread.ID, thread.Content); err != nil {
		return "", fmt.Errorf("failed to send message in thread: %w", err)
	}

	return createdThread.ID, nil
}

func (r *DiscordImpl) DeleteThread(threadID string) error {
	if r.session == nil {
		return ErrNotConfigured
	}

	_, err := r.session.ChannelDelete(threadID)
	if err != nil {
		return fmt.Errorf("failed to delete thread: %w", err)
	}

	return nil
}

func (r *DiscordImpl) SendMessage(channelID string, message string) error {
	if r.session == nil {
		return ErrNotConfigured
	}

	_, err := r.session.ChannelMessageSend(channelID, message)
	if err != nil {
		return fmt.Errorf("failed to send message to Discord: %w", err)
	}

	return nil
}
