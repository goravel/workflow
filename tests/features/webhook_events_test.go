package features

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/tests"
)

const (
	webhookSecret = "test-webhook-secret"
	webhookToken  = "test-api-token"

	commentDeliveryID = "2c9f4d1a-cb21-11f1-9f09-c29c45f23f62"
)

type WebhookEventsTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestWebhookEventsTestSuite(t *testing.T) {
	suite.Run(t, new(WebhookEventsTestSuite))
}

// SetupTest points the default sqlite connection at an isolated temp database
// and configures the github webhook secret / API token, then refreshes the
// application so the route handlers pick up the new configuration. The webhook
// library captures its secret when the handler is built, so a refresh is
// required for signature verification to use the test secret.
func (s *WebhookEventsTestSuite) SetupTest() {
	facades.Config().Add("database.default", "sqlite")
	facades.Config().Add("database.connections.sqlite.database",
		filepath.Join(s.T().TempDir(), "goravel_webhook_events_test.db"))
	facades.Config().Add("github", map[string]any{
		"webhook_secret": webhookSecret,
		"api_token":      webhookToken,
	})
	facades.App().Refresh()
	s.RefreshDatabase()
}

// signGitHub computes the X-Hub-Signature-256 value for the given payload.
func signGitHub(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// loadFixture reads a captured GitHub delivery from testdata.
func (s *WebhookEventsTestSuite) loadFixture(name string) string {
	body, err := os.ReadFile(filepath.Join("testdata", name))
	s.Require().NoError(err)

	return string(body)
}

func (s *WebhookEventsTestSuite) countEvents(includeTrashed bool) int64 {
	var events []models.WebhookEvent
	query := facades.Orm().Query().Model(&models.WebhookEvent{})
	if includeTrashed {
		query = query.WithTrashed()
	}
	s.NoError(query.Find(&events))

	return int64(len(events))
}

func (s *WebhookEventsTestSuite) seedEvent(platform, deliveryID, event, action, repository, sender string) models.WebhookEvent {
	webhookEvent := models.WebhookEvent{
		Platform:   platform,
		DeliveryID: deliveryID,
		Event:      event,
		Action:     action,
		Repository: repository,
		Sender:     sender,
		Payload:    "{}",
	}
	s.NoError(facades.Orm().Query().Create(&webhookEvent))

	return webhookEvent
}

func (s *WebhookEventsTestSuite) postWebhook(deliveryID, event, signature, body string) contractshttp.Response {
	request := s.Http(s.T()).
		WithHeader("X-GitHub-Event", event).
		WithHeader("X-GitHub-Delivery", deliveryID)
	if signature != "" {
		request = request.WithHeader("X-Hub-Signature-256", signature)
	}

	resp, err := request.Post("/webhooks/github", strings.NewReader(body))
	s.Require().NoError(err)

	return resp
}

func (s *WebhookEventsTestSuite) getEvents(uri string) contractshttp.Response {
	resp, err := s.Http(s.T()).WithHeader("X-Api-Token", webhookToken).Get(uri)
	s.Require().NoError(err)

	return resp
}

// Reject every webhook when no secret is configured.
func (s *WebhookEventsTestSuite) TestWebhookRejectedWhenSecretMissing() {
	facades.Config().Add("github", map[string]any{
		"webhook_secret": "",
		"api_token":      webhookToken,
	})
	facades.App().Refresh()

	body := `{"action":"opened"}`
	resp := s.postWebhook("delivery-no-secret", "push", signGitHub("whatever", body), body)
	resp.AssertUnauthorized()
	s.Equal(int64(0), s.countEvents(true))
}

// A real GitHub issue_comment/created delivery is stored verbatim and parsed
// into its metadata columns. Comment events are not dispatched.
func (s *WebhookEventsTestSuite) TestValidSignatureStoresEvent() {
	body := s.loadFixture("github_issue_comment_created.json")

	resp := s.postWebhook(commentDeliveryID, "issue_comment", signGitHub(webhookSecret, body), body)
	resp.AssertOk()

	var stored models.WebhookEvent
	s.NoError(facades.Orm().Query().Where("delivery_id", commentDeliveryID).First(&stored))
	s.Equal(models.PlatformGithub, stored.Platform)
	s.Equal("issue_comment", stored.Event)
	s.Equal("created", stored.Action)
	s.Equal("goravel/agents", stored.Repository)
	s.Equal("bowen", stored.Sender)
	s.Equal(body, stored.Payload)
	s.Contains(stored.Payload, "@goravel-agent")

	// The same delivery is stored only once, even when the redelivered payload
	// differs, and the original row is left untouched.
	changed := `{"action":"edited","repository":{"full_name":"other/repo"},"sender":{"login":"other"}}`
	resp = s.postWebhook(commentDeliveryID, "issue_comment", signGitHub(webhookSecret, changed), changed)
	resp.AssertOk()
	s.Equal(int64(1), s.countEvents(true))

	var unchanged models.WebhookEvent
	s.NoError(facades.Orm().Query().Where("delivery_id", commentDeliveryID).First(&unchanged))
	s.Equal(body, unchanged.Payload)
	s.Equal("goravel/agents", unchanged.Repository)
}

// A wrong or missing signature is rejected and nothing is stored.
func (s *WebhookEventsTestSuite) TestInvalidSignatureIsRejected() {
	body := `{"action":"opened"}`

	badSignature := s.postWebhook("delivery-bad", "push", "sha256=deadbeef", body)
	badSignature.AssertUnauthorized()

	missingSignature := s.postWebhook("delivery-missing", "push", "", body)
	missingSignature.AssertUnauthorized()

	s.Equal(int64(0), s.countEvents(true))
}

// An event type unknown to the library is still stored once the signature is valid.
func (s *WebhookEventsTestSuite) TestUnknownEventTypeIsStored() {
	body := `{"action":"created","repository":{"full_name":"goravel/framework"}}`

	resp := s.postWebhook("delivery-unknown", "custom_event_type", signGitHub(webhookSecret, body), body)
	resp.AssertOk()

	s.Equal(int64(1), s.countEvents(true))
	var stored models.WebhookEvent
	s.NoError(facades.Orm().Query().Where("delivery_id", "delivery-unknown").First(&stored))
	s.Equal("custom_event_type", stored.Event)
}

// An empty body is rejected with 401 and nothing is stored.
func (s *WebhookEventsTestSuite) TestEmptyBodyIsRejected() {
	resp := s.postWebhook("delivery-empty", "push", signGitHub(webhookSecret, ""), "")
	resp.AssertUnauthorized()
	s.Equal(int64(0), s.countEvents(true))
}

// A body larger than the cap is rejected and nothing is stored. The cap mirrors
// app/services/discord/github.go's maxWebhookBodyBytes (25 MB) and a non-JSON Content-Type keeps the
// HTTP driver's request wrapper from pre-consuming the body.
func (s *WebhookEventsTestSuite) TestOversizedBodyIsRejected() {
	const oversizeBodyBytes = 25<<20 + 1
	body := strings.Repeat("a", oversizeBodyBytes)

	resp, err := s.Http(s.T()).
		WithHeader("Content-Type", "text/plain").
		WithHeader("X-GitHub-Event", "push").
		WithHeader("X-GitHub-Delivery", "delivery-big").
		WithHeader("X-Hub-Signature-256", signGitHub(webhookSecret, body)).
		Post("/webhooks/github", strings.NewReader(body))
	s.Require().NoError(err)
	resp.AssertUnauthorized()
	s.Equal(int64(0), s.countEvents(true))
}

// A signed request without the event header is rejected and nothing is stored.
func (s *WebhookEventsTestSuite) TestMissingEventHeaderIsRejected() {
	body := `{"action":"opened"}`

	resp, err := s.Http(s.T()).
		WithHeader("X-GitHub-Delivery", "delivery-no-event").
		WithHeader("X-Hub-Signature-256", signGitHub(webhookSecret, body)).
		Post("/webhooks/github", strings.NewReader(body))
	s.Require().NoError(err)
	resp.AssertUnauthorized()
	s.Equal(int64(0), s.countEvents(true))
}

// A signed request without the delivery header is rejected and nothing is stored.
func (s *WebhookEventsTestSuite) TestMissingDeliveryHeaderIsRejected() {
	body := `{"action":"opened"}`

	resp, err := s.Http(s.T()).
		WithHeader("X-GitHub-Event", "push").
		WithHeader("X-Hub-Signature-256", signGitHub(webhookSecret, body)).
		Post("/webhooks/github", strings.NewReader(body))
	s.Require().NoError(err)
	resp.AssertUnauthorized()
	s.Equal(int64(0), s.countEvents(true))
}

// A non-JSON body with a valid signature is stored verbatim. A non-JSON
// Content-Type is used because the HTTP driver's request wrapper pre-parses and
// consumes application/json bodies it cannot decode.
func (s *WebhookEventsTestSuite) TestNonJSONBodyIsStoredVerbatim() {
	raw := "not-json"

	resp, err := s.Http(s.T()).
		WithHeader("Content-Type", "text/plain").
		WithHeader("X-GitHub-Event", "push").
		WithHeader("X-GitHub-Delivery", "delivery-raw").
		WithHeader("X-Hub-Signature-256", signGitHub(webhookSecret, raw)).
		Post("/webhooks/github", strings.NewReader(raw))
	s.Require().NoError(err)
	resp.AssertOk()

	var stored models.WebhookEvent
	s.NoError(facades.Orm().Query().Where("delivery_id", "delivery-raw").First(&stored))
	s.Equal(raw, stored.Payload)
}

// A pull_request/opened delivery is stored and acknowledged even with Discord
// off: no thread is opened and no tracking row is written.
func (s *WebhookEventsTestSuite) TestPullRequestOpenedWithoutDiscordStoresEvent() {
	facades.Config().Add("discord", map[string]any{
		"bot": map[string]any{"token": ""},
		"roles": map[string]any{
			"core": "1234567890",
		},
		"pull_requests": map[string]any{
			"channel_id": "9876543210",
		},
	})
	facades.App().Refresh()

	body := `{"action":"opened","number":7,` +
		`"pull_request":{"id":1234567,"title":"Add Discord guard","state":"open","draft":false,` +
		`"html_url":"https://github.com/goravel/agents/pull/7",` +
		`"user":{"login":"bowen","html_url":"https://github.com/bowen"}},` +
		`"repository":{"full_name":"goravel/agents","html_url":"https://github.com/goravel/agents"},` +
		`"sender":{"login":"bowen"}}`

	resp := s.postWebhook("delivery-pr-opened", "pull_request", signGitHub(webhookSecret, body), body)
	resp.AssertOk()

	var event models.WebhookEvent
	s.NoError(facades.Orm().Query().Where("delivery_id", "delivery-pr-opened").First(&event))
	s.Equal("opened", event.Action)
	s.Equal("goravel/agents", event.Repository)

	var rows []models.PullRequest
	s.NoError(facades.Orm().Query().Model(&models.PullRequest{}).Where("github_id", int64(1234567)).Find(&rows))
	s.Empty(rows)
}

// The read/delete endpoints are guarded by the API token.
func (s *WebhookEventsTestSuite) TestEventsEndpointsRequireToken() {
	// Missing header.
	getResp, err := s.Http(s.T()).Get("/webhooks/events")
	s.Require().NoError(err)
	getResp.AssertUnauthorized()

	// Wrong token.
	wrongResp, err := s.Http(s.T()).WithHeader("X-Api-Token", "wrong-token").Get("/webhooks/events")
	s.Require().NoError(err)
	wrongResp.AssertUnauthorized()

	deleteResp, err := s.Http(s.T()).Delete("/webhooks/events/1", nil)
	s.Require().NoError(err)
	deleteResp.AssertUnauthorized()

	// An empty configured token rejects every request, including an empty header.
	facades.Config().Add("github", map[string]any{
		"webhook_secret": webhookSecret,
		"api_token":      "",
	})
	emptyResp, err := s.Http(s.T()).WithHeader("X-Api-Token", "").Get("/webhooks/events")
	s.Require().NoError(err)
	emptyResp.AssertUnauthorized()
}

// Index paginates and filters by platform/event, newest first.
func (s *WebhookEventsTestSuite) TestIndexPaginatesAndFilters() {
	s.seedEvent(models.PlatformGithub, "d1", "push", "", "a/b", "x")
	s.seedEvent(models.PlatformGithub, "d2", "pull_request", "opened", "a/b", "x")
	s.seedEvent("gitlab", "d3", "push", "", "a/b", "x")

	resp := s.getEvents("/webhooks/events")
	resp.AssertOk()
	payload, err := resp.Json()
	s.Require().NoError(err)
	s.Equal(float64(3), payload["total"])
	s.Len(payload["data"], 3)
	s.Equal(float64(1), payload["page"])
	s.Equal(float64(50), payload["limit"])

	// Newest first.
	data, ok := payload["data"].([]any)
	s.Require().True(ok)
	first, ok := data[0].(map[string]any)
	s.Require().True(ok)
	s.Equal("gitlab", first["platform"])

	// Filter by event.
	pushResp := s.getEvents("/webhooks/events?event=push")
	pushJSON, err := pushResp.Json()
	s.Require().NoError(err)
	s.Equal(float64(2), pushJSON["total"])

	// Filter by platform.
	gitlabResp := s.getEvents("/webhooks/events?platform=gitlab")
	gitlabJSON, err := gitlabResp.Json()
	s.Require().NoError(err)
	s.Equal(float64(1), gitlabJSON["total"])

	// Pagination.
	pageResp := s.getEvents("/webhooks/events?page=1&limit=1")
	pageJSON, err := pageResp.Json()
	s.Require().NoError(err)
	s.Equal(float64(3), pageJSON["total"])
	s.Len(pageJSON["data"], 1)
	s.Equal(float64(1), pageJSON["limit"])

	// Bounds are clamped: page 0 -> 1, limit 501 -> 500, limit -1 -> 50.
	pageZeroResp := s.getEvents("/webhooks/events?page=0")
	pageZeroJSON, err := pageZeroResp.Json()
	s.Require().NoError(err)
	s.Equal(float64(1), pageZeroJSON["page"])

	limitMaxResp := s.getEvents("/webhooks/events?limit=501")
	limitMaxJSON, err := limitMaxResp.Json()
	s.Require().NoError(err)
	s.Equal(float64(500), limitMaxJSON["limit"])

	limitNegativeResp := s.getEvents("/webhooks/events?limit=-1")
	limitNegativeJSON, err := limitNegativeResp.Json()
	s.Require().NoError(err)
	s.Equal(float64(50), limitNegativeJSON["limit"])

	// Empty result returns an empty array, not null.
	emptyResp := s.getEvents("/webhooks/events?platform=none")
	emptyJSON, err := emptyResp.Json()
	s.Require().NoError(err)
	s.Equal(float64(0), emptyJSON["total"])
	s.Equal([]any{}, emptyJSON["data"])
}

// Destroy soft-deletes an event and hides it from Index.
func (s *WebhookEventsTestSuite) TestDestroySoftDeletesEvent() {
	stored := s.seedEvent(models.PlatformGithub, "d1", "push", "", "a/b", "x")

	deleteResp, err := s.Http(s.T()).
		WithHeader("X-Api-Token", webhookToken).
		Delete("/webhooks/events/"+strconv.FormatUint(uint64(stored.ID), 10), nil)
	s.Require().NoError(err)
	deleteResp.AssertNoContent(200)

	// Excluded from the (soft-delete filtering) list endpoint.
	listResp := s.getEvents("/webhooks/events")
	listJSON, err := listResp.Json()
	s.Require().NoError(err)
	s.Equal(float64(0), listJSON["total"])

	// Still present when trashed rows are included.
	s.Equal(int64(1), s.countEvents(true))

	// An absent id is a 404.
	missingResp, err := s.Http(s.T()).
		WithHeader("X-Api-Token", webhookToken).
		Delete("/webhooks/events/999999", nil)
	s.Require().NoError(err)
	missingResp.AssertNotFound()

	// A zero id is a 404.
	zeroResp, err := s.Http(s.T()).
		WithHeader("X-Api-Token", webhookToken).
		Delete("/webhooks/events/0", nil)
	s.Require().NoError(err)
	zeroResp.AssertNotFound()

	// A non-numeric id is a 404.
	nonNumericResp, err := s.Http(s.T()).
		WithHeader("X-Api-Token", webhookToken).
		Delete("/webhooks/events/not-a-number", nil)
	s.Require().NoError(err)
	nonNumericResp.AssertNotFound()
}
