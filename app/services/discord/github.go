package discord

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-playground/webhooks/v6/github"

	"goravel/app/facades"
	"goravel/app/models"
)

// maxWebhookBodyBytes caps the webhook request body read. GitHub allows
// payloads up to 25 MB, so anything larger is rejected before verification.
const maxWebhookBodyBytes = 25 << 20

// ErrMissingSecret is returned when no github webhook secret is configured, so
// no incoming webhook can be authenticated.
var ErrMissingSecret = errors.New("github webhook secret is not configured")

// ErrInvalidSignature is returned when a webhook payload cannot be
// authenticated (empty, oversized, missing, or wrong signature).
var ErrInvalidSignature = errors.New("invalid github webhook signature")

type Github interface {
	ProcessWebhook(request *http.Request) error
}

type GithubImpl struct {
	coreRoleID           string
	discord              Discord
	pullRequestChannelID string
	webhook              *github.Webhook
}

// newGithub wires the configured webhook secret and Discord channel settings
// into a GithubImpl around the supplied Discord dependency.
func newGithub(discord Discord) (*GithubImpl, error) {
	webhook, err := github.New(github.Options.Secret(facades.Config().GetString("github.webhook_secret")))
	if err != nil {
		return nil, err
	}

	return &GithubImpl{
		coreRoleID:           facades.Config().GetString("discord.roles.core"),
		discord:              discord,
		pullRequestChannelID: facades.Config().GetString("discord.pull_requests.channel_id"),
		webhook:              webhook,
	}, nil
}

func NewGithub() (*GithubImpl, error) {
	discord, err := NewDiscord()
	if err != nil {
		return nil, err
	}

	return newGithub(discord)
}

func (r *GithubImpl) ProcessWebhook(request *http.Request) error {
	if facades.Config().GetString("github.webhook_secret") == "" {
		return ErrMissingSecret
	}

	request.Body = http.MaxBytesReader(nil, request.Body, maxWebhookBodyBytes)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return ErrInvalidSignature
		}

		return err
	}
	if len(body) == 0 {
		return ErrInvalidSignature
	}
	// A delivery id is the dedupe key; a signed request without one would
	// otherwise collapse every later headerless delivery onto the same row.
	if request.Header.Get("X-GitHub-Delivery") == "" {
		return ErrInvalidSignature
	}
	// Restore the exact bytes so the library verifies + parses them below.
	request.Body = io.NopCloser(bytes.NewReader(body))

	// Passing the header event makes the library verify the signature for every
	// event type (otherwise it returns ErrEventNotFound before verifying).
	event, err := r.webhook.Parse(request, github.Event(request.Header.Get("X-GitHub-Event")))
	if err != nil {
		if isSignatureError(err) {
			return fmt.Errorf("%w: %w", ErrInvalidSignature, err)
		}

		// Signature passed; event type is unknown to the library. Store and stop.
		if storeErr := storeEvent(request, body); storeErr != nil {
			facades.Log().Error(fmt.Sprintf("failed to store webhook event: %v", storeErr))
		}

		return nil
	}

	if storeErr := storeEvent(request, body); storeErr != nil {
		facades.Log().Error(fmt.Sprintf("failed to store webhook event: %v", storeErr))
	}

	switch e := event.(type) {
	case github.PullRequestPayload:
		if e.Action == "opened" || e.Action == "reopened" {
			return r.handlePullRequestOpenedEvent(e)
		}

		pullRequest, err := r.getPullRequest(e.PullRequest.ID)
		if err != nil {
			return err
		}

		if pullRequest.ID == 0 {
			return nil
		}

		switch e.Action {
		case "ready_for_review":
			return r.handlePullRequestReadyForReviewEvent(pullRequest)
		// case "labeled":
		// 	return r.handlePullRequestLabeledEvent(e, pullRequest)
		case "closed":
			return r.handlePullRequestClosedEvent(pullRequest)
		}
	}

	return nil
}

func isSignatureError(err error) bool {
	return errors.Is(err, github.ErrHMACVerificationFailed) ||
		errors.Is(err, github.ErrMissingHubSignatureHeader) ||
		errors.Is(err, github.ErrMissingGithubEventHeader)
}

func storeEvent(request *http.Request, body []byte) error {
	var meta struct {
		Action     string `json:"action"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Sender struct {
			Login string `json:"login"`
		} `json:"sender"`
	}
	// Best-effort metadata extraction: an unparseable payload is still stored
	// verbatim, so the unmarshal error is intentionally ignored.
	_ = json.Unmarshal(body, &meta)

	event := models.WebhookEvent{
		Platform:   models.PlatformGithub,
		DeliveryID: request.Header.Get("X-GitHub-Delivery"),
		Event:      request.Header.Get("X-GitHub-Event"),
		Action:     meta.Action,
		Repository: meta.Repository.FullName,
		Sender:     meta.Sender.Login,
		Payload:    string(body),
	}

	// Match only the identity columns; every field is supplied as the insert
	// attribute so a redelivery with a different payload does not miss the row.
	lookup := &models.WebhookEvent{Platform: event.Platform, DeliveryID: event.DeliveryID}
	var existing models.WebhookEvent
	return facades.Orm().Query().WithTrashed().FirstOrCreate(&existing, lookup, &event)
}

// handlePullRequestOpenedEvent processes a github.PullRequestPayload.
func (r *GithubImpl) handlePullRequestOpenedEvent(payload github.PullRequestPayload) error {
	state := payload.PullRequest.State
	if payload.PullRequest.Draft {
		state = "Draft"
	}

	content := fmt.Sprintf("### 🛠️ New Pull Request Opened\n\n"+
		"Pull Request: [#%d - %s](%s)\n"+
		"Repository: [%s](%s)\n"+
		"Author: [%s](%s)\n"+
		"State: %s\n"+
		"Cc: <@&%s>",
		payload.Number,
		payload.PullRequest.Title,
		payload.PullRequest.HTMLURL,
		payload.Repository.FullName,
		payload.Repository.HTMLURL,
		payload.PullRequest.User.Login,
		payload.PullRequest.User.HTMLURL,
		state,
		r.coreRoleID)

	threadID, err := r.discord.CreateThread(r.pullRequestChannelID, Thread{
		Title:   payload.PullRequest.Title,
		Content: content,
	})
	// No thread means nothing to track, so skip the row as well.
	if errors.Is(err, ErrNotConfigured) {
		return nil
	}
	if err != nil {
		return err
	}

	var pullRequest models.PullRequest
	if err := facades.Orm().Query().Where("github_id", payload.PullRequest.ID).FirstOrCreate(&pullRequest, &models.PullRequest{
		DiscordThreadID: threadID,
		GithubID:        payload.PullRequest.ID,
		Title:           payload.PullRequest.Title,
		Url:             payload.PullRequest.HTMLURL,
	}); err != nil {
		return err
	}

	return nil
}

func (r *GithubImpl) handlePullRequestReadyForReviewEvent(pullRequest *models.PullRequest) error {
	err := r.discord.SendMessage(pullRequest.DiscordThreadID, fmt.Sprintf("Opend cc: <@&%s>", r.coreRoleID))
	if errors.Is(err, ErrNotConfigured) {
		return nil
	}

	return err
}

//nolint:unused
func (r *GithubImpl) handlePullRequestLabeledEvent(payload github.PullRequestPayload, pullRequest *models.PullRequest) error {
	for _, label := range payload.PullRequest.Labels {
		if strings.Contains(label.Name, "Review Ready") {
			return r.discord.SendMessage(pullRequest.DiscordThreadID, fmt.Sprintf("Review Ready cc: <@&%s>", r.coreRoleID))
		}
	}

	return nil
}

func (r *GithubImpl) handlePullRequestClosedEvent(pullRequest *models.PullRequest) error {
	// With Discord off there is no thread to delete, but the stored row still goes.
	if err := r.discord.DeleteThread(pullRequest.DiscordThreadID); err != nil && !errors.Is(err, ErrNotConfigured) {
		return err
	}

	if _, err := facades.Orm().Query().Delete(pullRequest); err != nil {
		return err
	}

	return nil
}

func (r *GithubImpl) getPullRequest(id int64) (*models.PullRequest, error) {
	var pullRequest models.PullRequest
	if err := facades.Orm().Query().Where("github_id", id).First(&pullRequest); err != nil {
		return nil, err
	}

	return &pullRequest, nil
}
