# Security

## Reporting a problem

The repository is private for now. Until it is public, report a security
problem to the owner, [chadmayfield](https://github.com/chadmayfield), through
GitHub.

Once the repository is public, GitHub's private vulnerability reporting will
be the route. Open the repository's Security tab and choose "Report a
vulnerability".

Please do not open a public issue for a security problem.

## How fz handles your token

- `fz` reads the token from the `FIZZY_TOKEN` environment variable, or runs
  the command in `FIZZY_TOKEN_CMD` or `token_cmd` in the config file and uses
  what it prints.
- `fz` never writes the token to disk and never prints it.
- The action log at `~/.local/state/fizzy-skill/actions.log` holds the time,
  method, path and status of each write. It does not hold the token, request
  bodies or response bodies.
- If the token command fails, the error says so and leaves out the command's
  output, because that output may hold the token.

The config file holds your server URL and account number, and the token
command, but not the token itself. Keep the token in a keychain or another
secret store, and restrict who can read the config file.

## Scope

`fz` sends the token to the server named in your config file or environment,
and to any pagination URL that server returns in a Link header.
`fz draft check --links` fetches the links in a draft without an
Authorization header.

`fz upload`, and `fz draft apply` for a draft with images, send each file's
bytes to the upload URL the server returns. That request carries no token,
but it goes wherever the server says.

`fz webhook create` prints the new webhook's signing secret once. In text
it goes to stderr, next to the warning to save it, so stdout stays free of
it. With `--json` it is in the JSON on stdout, because a script needs it. `fz webhook show --reveal-secret` prints it again. Anyone who
can read that output can sign requests as Fizzy, so keep it out of shared
logs.
