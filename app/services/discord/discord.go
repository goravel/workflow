package discord

import (
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"

	"goravel/app/facades"
)

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
	_, err := r.session.ChannelDelete(threadID)
	if err != nil {
		return fmt.Errorf("failed to delete thread: %w", err)
	}

	return nil
}

func (r *DiscordImpl) SendMessage(channelID string, message string) error {
	_, err := r.session.ChannelMessageSend(channelID, message)
	if err != nil {
		return fmt.Errorf("failed to send message to Discord: %w", err)
	}

	return nil
}
