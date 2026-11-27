package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleRelayResponse = `{
	"data": [
		{
			"id": 1,
			"created_at": "2026-10-11T10:00:00Z",
			"updated_at": "2026-10-11T10:00:00Z",
			"deleted_at": null,
			"platform": "github",
			"delivery_id": "delivery-1",
			"event": "issue_comment",
			"action": "created",
			"repository": "goravel/goravel",
			"sender": "bowen",
			"payload": "{\"number\":123,\"comment\":{\"id\":24680,\"body\":\"@goravel-agent review\"}}"
		},
		{
			"id": 2,
			"created_at": "2026-10-11T10:01:00Z",
			"updated_at": "2026-10-11T10:01:00Z",
			"deleted_at": null,
			"platform": "github",
			"delivery_id": "delivery-2",
			"event": "issues",
			"action": "opened",
			"repository": "goravel/framework",
			"sender": "other",
			"payload": "{\"number\":5,\"issue\":{\"number\":5,\"body\":\"no mention here\"}}"
		}
	],
	"total": 2,
	"page": 1,
	"limit": 50
}`

// installer88 is the goravel/installer#88 comment: the sentence explaining the
// change sits before the mention, so only `Any suggestions` used to reach the
// router and the agent.
const installer88 = "Sorry, missed it. The current module may be renamed to `project`, `pkg`, " +
	"or something else. Make it a breaking change. @goravel-agent Any suggestions?"

func TestParseEnvelope(t *testing.T) {
	env, err := parseEnvelope([]byte(sampleRelayResponse))
	require.NoError(t, err)
	require.Equal(t, 2, env.Total)
	require.Equal(t, 1, env.Page)
	require.Equal(t, 50, env.Limit)
	require.Len(t, env.Data, 2)

	require.Equal(t, uint64(1), env.Data[0].ID)
	require.Equal(t, "delivery-1", env.Data[0].DeliveryID)
	require.Equal(t, platformGitHub, env.Data[0].Platform)
	require.Equal(t, "goravel/goravel", env.Data[0].Repository)
}

func TestParseEnvelopeInvalid(t *testing.T) {
	_, err := parseEnvelope([]byte("not json"))
	require.Error(t, err)
}

// TestEventActionable pins which event/action pairs introduced the mention: the
// create/open actions plus a submitted pull request review, whose body is
// published exactly once.
func TestEventActionable(t *testing.T) {
	tests := []struct {
		name  string
		event event
		want  bool
	}{
		{name: "issue comment created", event: event{Event: "issue_comment", Action: "created"}, want: true},
		{name: "issue opened", event: event{Event: "issues", Action: "opened"}, want: true},
		{name: "review submitted", event: event{Event: eventPullRequestReview, Action: eventActionSubmitted}, want: true},
		{name: "review edited", event: event{Event: eventPullRequestReview, Action: "edited"}},
		{name: "review dismissed", event: event{Event: eventPullRequestReview, Action: "dismissed"}},
		{name: "issue labeled", event: event{Event: "issues", Action: "labeled"}},
		{name: "submitted on another event", event: event{Event: "pull_request", Action: eventActionSubmitted}},
		{name: "no action", event: event{Event: eventPullRequestReview}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.event.actionable())
		})
	}
}

func TestEventInstruction(t *testing.T) {
	env, err := parseEnvelope([]byte(sampleRelayResponse))
	require.NoError(t, err)

	instruction, ok := env.Data[0].instruction(DefaultMention)
	require.True(t, ok)
	require.Equal(t, "@goravel-agent review", instruction)

	_, ok = env.Data[1].instruction(DefaultMention)
	require.False(t, ok)
}

func TestEventInstructionFallbacks(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		payload string
		want    string
		ok      bool
	}{
		{
			name:    "comment body",
			payload: `{"number":1,"comment":{"id":9,"body":"@goravel-agent plan this"}}`,
			want:    "@goravel-agent plan this",
			ok:      true,
		},
		{
			name:    "issue body",
			payload: `{"number":2,"issue":{"number":2,"body":"@goravel-agent ship"}}`,
			want:    "@goravel-agent ship",
			ok:      true,
		},
		{
			name:    "pull request body",
			payload: `{"number":3,"pull_request":{"number":3,"body":"@goravel-agent review please"}}`,
			want:    "@goravel-agent review please",
			ok:      true,
		},
		{
			name:    "review body",
			event:   eventPullRequestReview,
			payload: `{"pull_request":{"number":170},"review":{"id":5778727749,"node_id":"MDE3OlB1bGxSZXF1ZXN0UmV2aWV3ODA=","body":"@goravel-agent solve the comments"}}`,
			want:    "@goravel-agent solve the comments",
			ok:      true,
		},
		{
			name:    "review body never falls through to the pull request body",
			event:   eventPullRequestReview,
			payload: `{"pull_request":{"number":170,"body":"@goravel-agent review"}}`,
			ok:      false,
		},
		{
			name:    "mention without instruction",
			payload: `{"number":4,"comment":{"id":1,"body":"hi @goravel-agent"}}`,
			want:    "hi @goravel-agent",
			ok:      false,
		},
		{
			name:    "longer handle is not a mention",
			payload: `{"number":4,"comment":{"id":1,"body":"@goravel-agent2 review"}}`,
			ok:      false,
		},
		{
			name:    "prefixed word is not a mention",
			payload: `{"number":4,"comment":{"id":1,"body":"ping bob@goravel-agent.com please"}}`,
			ok:      false,
		},
		{
			name:    "hyphen before handle is not a mention",
			payload: `{"number":4,"comment":{"id":1,"body":"ping bob-@goravel-agent.com please"}}`,
			ok:      false,
		},
		{
			name:    "hyphenated suffix is not a mention",
			payload: `{"number":4,"comment":{"id":1,"body":"@goravel-agent-review please"}}`,
			ok:      false,
		},
		{
			name:    "pluralised handle is not a mention",
			payload: `{"number":4,"comment":{"id":1,"body":"@goravel-agents review"}}`,
			ok:      false,
		},
		{
			name:    "org prefix alone is not a mention",
			payload: `{"number":4,"comment":{"id":1,"body":"@goravel review"}}`,
			ok:      false,
		},
		{
			name:    "underscored handle is not a mention",
			payload: `{"number":4,"comment":{"id":1,"body":"@goravel_agent review"}}`,
			ok:      false,
		},
		{
			name:    "space separated handle is not a mention",
			payload: `{"number":4,"comment":{"id":1,"body":"@goravel agent review"}}`,
			ok:      false,
		},
		{
			name:    "valid mention after a rejected one",
			payload: `{"number":4,"comment":{"id":1,"body":"@goravel-agent2 then @goravel-agent review"}}`,
			want:    "@goravel-agent2 then @goravel-agent review",
			ok:      true,
		},
		{
			name:    "handle followed by punctuation",
			payload: `{"number":4,"comment":{"id":1,"body":"@goravel-agent, please review"}}`,
			want:    "@goravel-agent, please review",
			ok:      true,
		},
		{
			name:    "bare decorated mention is not an instruction",
			payload: `{"number":4,"comment":{"id":1,"body":"**@goravel-agent**"}}`,
			want:    "**@goravel-agent**",
			ok:      false,
		},
		{
			name:    "case-insensitive handle",
			payload: `{"number":4,"comment":{"id":1,"body":"@Goravel-Agent review"}}`,
			want:    "@Goravel-Agent review",
			ok:      true,
		},
		{
			// goravel/installer#88: the sentence before the mention is the context
			// the router and the agent were losing.
			name:    "whole comment with its mention",
			payload: `{"number":88,"comment":{"id":1,"body":"` + installer88 + `"}}`,
			want:    installer88,
			ok:      true,
		},
		{
			name:    "mention mid-sentence keeps the sentence",
			payload: `{"number":6,"comment":{"id":1,"body":"The rename is breaking. @goravel-agent what about the config key?"}}`,
			want:    "The rename is breaking. @goravel-agent what about the config key?",
			ok:      true,
		},
		{
			name:    "paragraphs stay verbatim",
			payload: `{"number":7,"comment":{"id":1,"body":"First paragraph.\n\n@goravel-agent please check the rename.\n\nSecond paragraph."}}`,
			want:    "First paragraph.\n\n@goravel-agent please check the rename.\n\nSecond paragraph.",
			ok:      true,
		},
		{
			name:    "outer whitespace is trimmed",
			payload: `{"number":8,"comment":{"id":1,"body":"  @goravel-agent review  "}}`,
			want:    "@goravel-agent review",
			ok:      true,
		},
		{
			name:    "malformed payload",
			payload: `{`,
			ok:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &event{Event: test.event, Payload: test.payload}
			got, ok := event.instruction(DefaultMention)
			require.Equal(t, test.ok, ok)
			require.Equal(t, test.want, got)
		})
	}
}

func TestInstructionEmptyMention(t *testing.T) {
	event := &event{Payload: `{"comment":{"id":1,"body":"hello world"}}`}

	_, ok := event.instruction("")
	require.False(t, ok)
}

func TestEventTarget(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    int
	}{
		{name: "top-level number", payload: `{"number":123}`, want: 123},
		{name: "issue number", payload: `{"issue":{"number":7}}`, want: 7},
		{name: "pull request number", payload: `{"pull_request":{"number":42}}`, want: 42},
		{name: "review pull request number", payload: `{"pull_request":{"number":170},"review":{"id":5778727749}}`, want: 170},
		{name: "top-level wins", payload: `{"number":5,"issue":{"number":7}}`, want: 5},
		{name: "none", payload: `{}`, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &event{Payload: test.payload}
			require.Equal(t, test.want, event.target())
		})
	}
}

// TestEventDefaultTitle pins the fallback run title built from the event: a pull
// request yields `PR #<n>`, an issue `Issue #<n>`, and a payload without a
// number yields no title at all.
func TestEventDefaultTitle(t *testing.T) {
	tests := []struct {
		name  string
		event event
		want  string
	}{
		{name: "pull request number", event: event{Repository: "goravel/framework", Payload: `{"number":42,"pull_request":{"number":42}}`}, want: "PR #42"},
		{name: "issue number", event: event{Repository: "goravel/goravel", Payload: `{"number":123,"comment":{"body":"@goravel-agent review"}}`}, want: "Issue #123"},
		{name: "issue number only", event: event{Repository: "goravel/goravel", Payload: `{"issue":{"number":7,"body":"@goravel-agent review"}}`}, want: "Issue #7"},
		{name: "review pull request", event: event{Repository: "goravel/example", Payload: `{"pull_request":{"number":170,"body":"A change"},"review":{"id":5778727749}}`}, want: "PR #170"},
		{name: "no number", event: event{Repository: "goravel/goravel", Payload: `{"comment":{"body":"@goravel-agent review"}}`}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.event.defaultTitle())
		})
	}
}

// TestEventReaction pins which comment the eyes reaction targets: conversation
// comments go through the issues collection, inline review comments through the
// pull request collection, and a body-only mention has no comment at all.
func TestEventReaction(t *testing.T) {
	tests := []struct {
		name           string
		event          string
		payload        string
		wantCollection string
		wantID         int64
		wantOK         bool
	}{
		{
			name:           "conversation comment",
			event:          "issue_comment",
			payload:        `{"number":123,"comment":{"id":24680,"body":"@goravel-agent review"}}`,
			wantCollection: reactionCollectionIssue,
			wantID:         24680,
			wantOK:         true,
		},
		{
			name:           "inline review comment",
			event:          "pull_request_review_comment",
			payload:        `{"pull_request":{"number":123},"comment":{"id":98765,"body":"@goravel-agent review"}}`,
			wantCollection: reactionCollectionReview,
			wantID:         98765,
			wantOK:         true,
		},
		{
			name:    "issue body only",
			event:   "issues",
			payload: `{"number":5,"issue":{"number":5,"body":"@goravel-agent review"}}`,
		},
		{
			name:    "review body only",
			event:   eventPullRequestReview,
			payload: `{"pull_request":{"number":170},"review":{"id":5778727749,"node_id":"MDE3OlB1bGxSZXF1ZXN0UmV2aWV3ODA=","body":"@goravel-agent solve the comments"}}`,
		},
		{
			name:    "invalid payload",
			event:   "issue_comment",
			payload: `not json`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &event{Event: test.event, Payload: test.payload}

			collection, commentID, ok := event.reaction()
			require.Equal(t, test.wantOK, ok)
			require.Equal(t, test.wantCollection, collection)
			require.Equal(t, test.wantID, commentID)
		})
	}
}

// TestEventReviewNodeID pins which event carries a review reaction subject: a
// review body has one, every other event — including an inline review comment —
// has none.
func TestEventReviewNodeID(t *testing.T) {
	tests := []struct {
		name  string
		event event
		want  string
	}{
		{
			name:  "review body",
			event: event{Event: eventPullRequestReview, Payload: `{"review":{"id":5778727749,"node_id":"MDE3OlB1bGxSZXF1ZXN0UmV2aWV3ODA="}}`},
			want:  "MDE3OlB1bGxSZXF1ZXN0UmV2aWV3ODA=",
		},
		{
			name:  "review without node id",
			event: event{Event: eventPullRequestReview, Payload: `{"review":{"id":5778727749}}`},
		},
		{
			name:  "inline review comment",
			event: event{Event: "pull_request_review_comment", Payload: `{"comment":{"id":98765,"body":"@goravel-agent review"}}`},
		},
		{
			name:  "conversation comment",
			event: event{Event: "issue_comment", Payload: `{"comment":{"id":24680,"body":"@goravel-agent review"}}`},
		},
		{
			name:  "review object on another event",
			event: event{Event: eventPullRequestReviewComment, Payload: `{"comment":{"id":98765,"body":"@goravel-agent review"},"review":{"id":5778727749,"node_id":"MDE3OlB1bGxSZXF1ZXN0UmV2aWV3ODA="}}`},
		},
		{
			name:  "invalid payload",
			event: event{Event: eventPullRequestReview, Payload: "not json"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.event.reviewNodeID())
		})
	}
}

func TestEventPrompt(t *testing.T) {
	event := &event{
		Event:      "issue_comment",
		Repository: "goravel/goravel",
		Payload: `{"number":123,` +
			`"pull_request":{"number":123,"html_url":"https://github.com/goravel/goravel/pull/123"},` +
			`"comment":{"id":24680,"html_url":"https://github.com/goravel/goravel/pull/123#issuecomment-24680",` +
			`"body":"@goravel-agent review"}}`,
	}

	want := "Repository: goravel/goravel\n" +
		"PR: https://github.com/goravel/goravel/pull/123\n" +
		"comment: https://github.com/goravel/goravel/pull/123#issuecomment-24680\n" +
		"Instruction: @goravel-agent review"
	require.Equal(t, want, event.prompt("@goravel-agent review", "goravel/goravel"))
}

// TestEventPromptKeepsTheWholeComment pins goravel/installer#88: the router and
// the dispatched agent get the whole comment that carried the mention, handle
// included, so the sentence explaining the change is not lost.
func TestEventPromptKeepsTheWholeComment(t *testing.T) {
	event := &event{
		Event:      "issue_comment",
		Repository: "goravel/installer",
		Payload: `{"number":88,"comment":{"id":1061,` +
			`"html_url":"https://github.com/goravel/installer/issues/88#issuecomment-1061",` +
			`"body":"` + installer88 + `"}}`,
	}

	instruction, ok := event.instruction(DefaultMention)
	require.True(t, ok)
	require.Equal(t, installer88, instruction)

	want := "Instruction: " + installer88

	require.Equal(t,
		"Repository: goravel/installer\n"+
			"Issue: https://github.com/goravel/installer/issues/88\n"+
			"comment: https://github.com/goravel/installer/issues/88#issuecomment-1061\n"+
			want,
		event.prompt(instruction, "goravel/installer"))

	require.Contains(t, event.contextBlock(instruction), want)
}

// TestEventAgentPrompt pins the agent prompt as the event context verbatim: the
// same links as the router prompt, no Repository line, and the instruction
// exactly as written, with no trusted line appended for any agent.
func TestEventAgentPrompt(t *testing.T) {
	tests := []struct {
		name        string
		event       string
		payload     string
		instruction string
		want        string
	}{
		{
			name:        "pull request comment",
			event:       "issue_comment",
			payload:     `{"number":123,"pull_request":{"number":123,"html_url":"https://github.com/goravel/goravel/pull/123"},"comment":{"id":24680,"html_url":"https://github.com/goravel/goravel/pull/123#issuecomment-24680","body":"@goravel-agent review"}}`,
			instruction: "@goravel-agent review",
			want: "PR: https://github.com/goravel/goravel/pull/123\n" +
				"comment: https://github.com/goravel/goravel/pull/123#issuecomment-24680\n" +
				"Instruction: @goravel-agent review",
		},
		{
			name:        "pull request description",
			event:       "issue_comment",
			payload:     `{"issue":{"number":45,"html_url":"https://github.com/goravel/goravel/pull/45","pull_request":{"html_url":"https://github.com/goravel/goravel/pull/45"},"body":"@goravel-agent review"}}`,
			instruction: "@goravel-agent review",
			want: "PR: https://github.com/goravel/goravel/pull/45\n" +
				"Instruction: @goravel-agent review",
		},
		{
			name:        "issue comment",
			event:       "issue_comment",
			payload:     `{"number":45,"comment":{"id":24680,"body":"@goravel-agent review"}}`,
			instruction: "@goravel-agent review",
			want: "Issue: https://github.com/goravel/goravel/issues/45\n" +
				"comment: https://github.com/goravel/goravel/issues/45#issuecomment-24680\n" +
				"Instruction: @goravel-agent review",
		},
		{
			name:        "multi-line instruction",
			event:       "issue_comment",
			payload:     `{"number":123,"pull_request":{"number":123,"html_url":"https://github.com/goravel/goravel/pull/123"},"comment":{"id":24680,"html_url":"https://github.com/goravel/goravel/pull/123#issuecomment-24680","body":"@goravel-agent review\nthen post it"}}`,
			instruction: "@goravel-agent review\nthen post it",
			want: "PR: https://github.com/goravel/goravel/pull/123\n" +
				"comment: https://github.com/goravel/goravel/pull/123#issuecomment-24680\n" +
				"Instruction: @goravel-agent review\nthen post it",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &event{Event: test.event, Repository: "goravel/goravel", Payload: test.payload}

			got := event.contextBlock(test.instruction)
			require.Equal(t, test.want, got)
			require.NotContains(t, got, "Repository:")
			require.NotContains(t, got, "comment the review result to the pr")
		})
	}
}

// TestEventPromptReview pins the review prompt: the PR link plus the review
// anchor, labelled `comment:` so the flows' acknowledge step still finds a link
// to swap.
func TestEventPromptReview(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "review html_url",
			payload: `{"pull_request":{"number":170,"html_url":"https://github.com/goravel/example/pull/170"},` +
				`"review":{"id":5778727749,"node_id":"MDE3OlB1bGxSZXF1ZXN0UmV2aWV3ODA=",` +
				`"html_url":"https://github.com/goravel/example/pull/170#pullrequestreview-5778727749",` +
				`"body":"@goravel-agent solve the comments"}}`,
		},
		{
			name: "review without html_url",
			payload: `{"pull_request":{"number":170},` +
				`"review":{"id":5778727749,"node_id":"MDE3OlB1bGxSZXF1ZXN0UmV2aWV3ODA=",` +
				`"body":"@goravel-agent solve the comments"}}`,
		},
	}

	want := "Repository: goravel/example\n" +
		"PR: https://github.com/goravel/example/pull/170\n" +
		"comment: https://github.com/goravel/example/pull/170#pullrequestreview-5778727749\n" +
		"Instruction: @goravel-agent solve the comments"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &event{Event: eventPullRequestReview, Repository: "goravel/example", Payload: test.payload}
			require.Equal(t, want, event.prompt("@goravel-agent solve the comments", "goravel/example"))
		})
	}
}

// TestEventPromptIgnoresReviewOnOtherEvents pins that review fields are read only
// on a review event: an inline review comment keeps its own #discussion_r link
// even when the payload carries a review object.
func TestEventPromptIgnoresReviewOnOtherEvents(t *testing.T) {
	event := &event{
		Event:      eventPullRequestReviewComment,
		Repository: "goravel/example",
		Payload: `{"pull_request":{"number":170,"html_url":"https://github.com/goravel/example/pull/170"},` +
			`"comment":{"id":98765,"html_url":"https://github.com/goravel/example/pull/170#discussion_r98765",` +
			`"body":"@goravel-agent review"},"review":{"id":5778727749,` +
			`"html_url":"https://github.com/goravel/example/pull/170#pullrequestreview-5778727749"}}`,
	}

	want := "Repository: goravel/example\n" +
		"PR: https://github.com/goravel/example/pull/170\n" +
		"comment: https://github.com/goravel/example/pull/170#discussion_r98765\n" +
		"Instruction: @goravel-agent review"
	require.Equal(t, want, event.prompt("@goravel-agent review", "goravel/example"))
}

func TestEventPromptWithoutComment(t *testing.T) {
	event := &event{
		Event:      "issues",
		Repository: "goravel/framework",
		Payload: `{"issue":{"number":5,"html_url":"https://github.com/goravel/framework/issues/5",` +
			`"body":"@goravel-agent plan"}}`,
	}

	want := "Repository: goravel/framework\n" +
		"Issue: https://github.com/goravel/framework/issues/5\n" +
		"Instruction: @goravel-agent plan"
	require.Equal(t, want, event.prompt("@goravel-agent plan", "goravel/framework"))
}

// TestEventPromptTargetRepository pins that the Repository line shows the
// router-chosen target while the links keep pointing at the source repository
// where the mention was posted.
func TestEventPromptTargetRepository(t *testing.T) {
	event := &event{
		Event:      "issues",
		Repository: "goravel/goravel",
		Payload: `{"issue":{"number":45,"html_url":"https://github.com/goravel/goravel/issues/45",` +
			`"body":"@goravel-agent plan"}}`,
	}

	want := "Repository: goravel/framework\n" +
		"Issue: https://github.com/goravel/goravel/issues/45\n" +
		"Instruction: @goravel-agent plan"
	require.Equal(t, want, event.prompt("@goravel-agent plan", "goravel/framework"))
}

// TestEventPromptMutuallyExclusiveLinks pins that a pull request shows only the
// PR link and an issue shows only the Issue link.
func TestEventPromptMutuallyExclusiveLinks(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantLink    string
		wantComment string
		omitIssues  bool
		omitPR      bool
	}{
		{
			name: "pull request",
			payload: `{"pull_request":{"number":123,"html_url":"https://github.com/goravel/goravel/pull/123"},` +
				`"comment":{"id":1,"body":"@goravel-agent review"}}`,
			wantLink:    "PR: https://github.com/goravel/goravel/pull/123\n",
			wantComment: "comment: https://github.com/goravel/goravel/pull/123#issuecomment-1\n",
			omitIssues:  true,
		},
		{
			name: "issue",
			payload: `{"issue":{"number":45,"html_url":"https://github.com/goravel/goravel/issues/45",` +
				`"body":"@goravel-agent review"}}`,
			wantLink: "Issue: https://github.com/goravel/goravel/issues/45\n",
			omitPR:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &event{
				Repository: "goravel/goravel",
				Payload:    test.payload,
			}

			got := event.prompt("review", "goravel/goravel")
			require.Contains(t, got, test.wantLink)
			if test.wantComment != "" {
				require.Contains(t, got, test.wantComment)
			}
			if test.omitIssues {
				require.NotContains(t, got, "Issue:")
			}
			if test.omitPR {
				require.NotContains(t, got, "PR:")
			}
		})
	}
}

// TestEventPromptPullRequestLinks covers the two PR shapes that carry no
// top-level pull_request.html_url: an issue_comment on a pull request (nested
// issue.pull_request) and a pull request with no html_url at all (constructed
// fallback).
func TestEventPromptPullRequestLinks(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name: "issue comment on a pull request",
			payload: `{"issue":{"number":45,"html_url":"https://github.com/goravel/goravel/pull/45",` +
				`"pull_request":{"html_url":"https://github.com/goravel/goravel/pull/45"},` +
				`"body":"@goravel-agent review"}}`,
			want: "Repository: goravel/goravel\n" +
				"PR: https://github.com/goravel/goravel/pull/45\n" +
				"Instruction: @goravel-agent review",
		},
		{
			name:    "pull request without html_url",
			payload: `{"pull_request":{"number":123},"comment":{"id":24680,"body":"@goravel-agent review"}}`,
			want: "Repository: goravel/goravel\n" +
				"PR: https://github.com/goravel/goravel/pull/123\n" +
				"comment: https://github.com/goravel/goravel/pull/123#issuecomment-24680\n" +
				"Instruction: @goravel-agent review",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &event{Repository: "goravel/goravel", Payload: test.payload}
			require.Equal(t, test.want, event.prompt("@goravel-agent review", "goravel/goravel"))
		})
	}
}

// TestEventPromptFallbackLinks pins the constructed links used when the payload
// carries no html_url fields.
func TestEventPromptFallbackLinks(t *testing.T) {
	event := &event{
		Event:      "issue_comment",
		Repository: "goravel/goravel",
		Payload:    `{"number":45,"comment":{"id":24680,"body":"@goravel-agent review"}}`,
	}

	want := "Repository: goravel/goravel\n" +
		"Issue: https://github.com/goravel/goravel/issues/45\n" +
		"comment: https://github.com/goravel/goravel/issues/45#issuecomment-24680\n" +
		"Instruction: @goravel-agent review"
	require.Equal(t, want, event.prompt("@goravel-agent review", "goravel/goravel"))
}

// TestEventPromptReviewCommentFallbackLink pins the synthesized mention link for
// an inline review comment with no stored html_url: it is anchored with
// #discussion_r so the run answers in that thread, and no link is built when the
// pull request number is unknown.
func TestEventPromptReviewCommentFallbackLink(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name: "discussion anchor",
			payload: `{"pull_request":{"number":170},` +
				`"comment":{"id":98765,"body":"@goravel-agent review"}}`,
			want: "comment: https://github.com/goravel/example/pull/170#discussion_r98765",
		},
		{
			name:    "no pull request number",
			payload: `{"comment":{"id":98765,"body":"@goravel-agent review"}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &event{Event: eventPullRequestReviewComment, Repository: "goravel/example", Payload: test.payload}
			got := event.prompt("review", "goravel/example")

			if test.want != "" {
				require.Contains(t, got, test.want)
				require.NotContains(t, got, "#issuecomment-")
				return
			}
			require.NotContains(t, got, "comment:")
		})
	}
}
