# Release

The package releases major and patch versions for framework and packages, and runs the `@goravel-agent` mention poller. The release process is very complex, so we built this package to save time.

## Setup

Set the `GITHUB_TOKEN` environment variable in the `.env` file to your GitHub token first. Required Read and Write accesses: Contents, Pull requests, and Actions.

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

Set `AGENT_BASE_URL`, `AGENT_API_TOKEN`, `AGENT_ALLOWED_SENDERS`,
`AGENT_PROJECT_NAME` and `AI_PROVIDER`/`DEEPSEEK_API_KEY` in `.env` (see
`.env.example`); `GITHUB_TOKEN` enables the 👀 reaction. The `paseo` CLI must be
on `PATH`. The mode definitions (`ship`, `review`, `analyze`) still live in the
`goravel/agents` repository.

```shell
# One poll cycle
./artisan poll-events

# Scheduler: poll-events every five seconds, skipping an overlapping tick
./artisan schedule:run
```

The live-router feature tests skip themselves unless `AI_PROVIDER` and its API
key are set.
