# Release

The package releases major and patch versions for framework and packages, and runs the `@goravel-agent` mention poller. The release process is very complex, so we built this package to save time.

## Setup

Copy `.env.example` to `.env` and fill in only the parts you need — the file is grouped into **Save GitHub events**, **Consume GitHub events**, **Notification in Discord**, and shared-key comments. The unlabelled keys at the top are common to every feature: app name, key, debug flag, logging, the sqlite database, and session settings.

Release Goravel version has no section of its own: the release commands read `GITHUB_TOKEN` from the shared-key block, whose comment names Release Goravel version.

The **Notification in Discord** section is optional: leave every `DISCORD_*` and `MAIL_*` key blank and webhook events are still stored — only pull-request threads, core-role mentions, heartbeat alerts, and webhook failure mail are skipped. `MAIL_*` is the failure-alert channel, so it lives here even though the section is named Discord.

The release commands need a token with Read and Write access to Contents, Pull requests, and Actions, plus Read access to Checks.

Github link: https://github.com/settings/personal-access-tokens

## Usage

There are three main commands: `preview`, `major`, and `patch`.

1. Preview the release information

It's useful to preview the changes when generating the documentation.

```
# Preview framework and goravel-lite changes only
./artisan preview v1.16.0

# Preview all packages changes
./artisan preview v1.16.0 --packages
```

2. Release major version

The command will release the major version for framework and all sub-packages.

```
# Preview mode (default)
./artisan major v1.16.0

# Real release
./artisan major v1.16.0 --real

# With optional flags
./artisan major v1.16.0 --real --refresh --framework-branch=custom-branch
```

Available flags for major release:
- `--real`, `-r`: Perform actual release (without this flag, it's preview mode)
- `--refresh`: Refresh Go module proxy cache before release
- `--framework-branch`, `-fb`: Specify framework branch (useful when go mod cannot fetch the latest master)

3. Release patch version

The command will release the patch version for framework and example, goravel-lite, goravel.

```
# Preview mode (default)
./artisan patch v1.15.1

# Real release
./artisan patch v1.15.1 --real
```

## Testing

Run command below to run test:

```
go test ./...
```

## Mention agent

The repo also runs the `@goravel-agent` mention poller: it reads relay events,
routes each mention to a `paseo` mode (`ship`, `review`, `analyze`) with the
configured AI provider, and dispatches the run.

Fill in **Consume GitHub events** in `.env.example` (`AGENT_BASE_URL`,
`AGENT_PROJECT_NAME`, `AGENT_ALLOWED_SENDERS`, `AI_PROVIDER`/`DEEPSEEK_API_KEY`);
`AGENT_PROJECT_NAME` and `AGENT_ALLOWED_SENDERS` are required — the `paseo`
project each run's workspace is created in, and a comma-separated list of
GitHub usernames such as `bowen, alice`. `poll-events` refuses to start when
either is blank. `AGENT_API_TOKEN` is shared with Save GitHub events and
`GITHUB_TOKEN` is shared with Release Goravel version — both are declared in
the shared-key block. When consuming events the `GITHUB_TOKEN` is optional and
only enables the 👀 reaction. The `paseo` CLI must be on `PATH`. The mode
definitions (`ship`, `review`, `analyze`) still live in the `goravel/agents`
repository.

```shell
# One poll cycle
./artisan poll-events

# Scheduler: poll-events every five seconds, skipping an overlapping tick
./artisan schedule:run
```

The live-router feature tests skip themselves unless `AI_PROVIDER` and its API
key are set.
