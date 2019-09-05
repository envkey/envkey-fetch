# envkey-fetch

This library contains [EnvKey](https://www.envkey.com)'s core cross-platform fetching, decryption, verification, web of trust, redundancy, and caching logic. It accepts an `ENVKEY` generated by the [EnvKey App](https://www.github.com/envkey/envkey-app) and returns decrypted configuration for a specific app environment as json.

It is used by EnvKey's various Client Libraries, including [envkey-source](https://github.com/envkey/envkey-source) for bash, [envkey-ruby](https://github.com/envkey/envkey-ruby) for Ruby and Rails, [envkey-python](https://github.com/envkey/envkey-python) for Python, [envkey-node](https://github.com/envkey/envkey-node) for Node.js, and [envkeygo](https://github.com/envkey/envkeygo) for Go.

If you want to build an EnvKey library in a language that isn't yet officially supported, build some other type of integration, or simply play around with EnvKey on the command line, envkey-fetch is the library for you. If you just want to integrate EnvKey with your project, check out one of the aforementioned higher level libraries.

## Installation

envkey-fetch compiles into a simple static binary with no dependencies, which makes installation a simple matter of fetching the right binary for your platform and putting it in your `PATH`. An `install.sh` script is available to simplify this, as well as a [homebrew tap](https://github.com/envkey/homebrew-envkey)..

**Install via bash:**

```bash
curl -s https://raw.githubusercontent.com/envkey/envkey-fetch/master/install.sh | bash
```

**Install via [homebrew](https://brew.sh/) tap:**

Either tap the [homebrew-envkey](https://github.com/envkey/homebrew-envkey) repo first, then install:

```bash
brew tap envkey/envkey
brew install envkey-fetch
```

Or you can install the formula directly:

```bash
brew install envkey/envkey/envkey-fetch
```

**Install manually:**

Find the [release](https://github.com/envkey/envkey-fetch/releases) for your platform and architecture, and stick the appropriate binary somewhere in your `PATH` (or wherever you like really).

**Install from source:**

With Go installed, clone the project into your `GOPATH`. `cd` into the directory and run `go get` and `go build`.

**Cross-compile from source:**

To compile cross-platform binaries, make sure Go is installed, then install [goreleaser](https://goreleaser.com/) - follow instructions in the docs to do so.

Then to cross-compile, run:

`goreleaser`

Binaries for each platform will be output to the `dist` folder.

## Usage

```bash
envkey-fetch YOUR-ENVKEY [flags]
```

This will either write your the app environment's configuration associated with your `ENVKEY` as json to stdout or write an error message beginning with `error:` to stdout.

### Example json output

```json
{"TEST":"it","TEST_2":"works!"}
```

### Example error output

```text
error: ENVKEY invalid
```

### Flags

```text
    --cache                   cache encrypted config as a local backup (default is false)
    --cache-dir string        cache directory (default is $HOME/.envkey/cache)
    --client-name string      calling client library name (default is none)
    --client-version string   calling client library version (default is none)
-h, --help                    help for envkey-fetch
    --retries uint8           number of times to retry requests on failure (default 3)
    --retryBackoff float      retry backoff factor: {retryBackoff} * (2 ^ {retries - 1}) (default 1)
    --timeout float           timeout in seconds for http requests (default 10)
    --verbose                 print verbose output (default is false)
-v, --version                 prints the version
```

## x509 error / ca-certificates

On a stripped down OS like Alpine Linux, you may get an `x509: certificate signed by unknown authority` error when `envkey-fetch` attempts to load your config. `envkey-fetch` tries to handle this by including its own set of trusted CAs via [gocertifi](https://github.com/certifi/gocertifi), but if you're getting this error anyway, you can fix it by ensuring that the `ca-certificates` dependency is installed. On Alpine you'll want to run:
```
apk add --no-cache ca-certificates
```

## Further Reading

For more on EnvKey in general:

Read the [docs](https://docs.envkey.com).

Read the [integration quickstart](https://docs.envkey.com/integration-quickstart.html).

Read the [security and cryptography overview](https://security.envkey.com).

## Need help? Have questions, feedback, or ideas?

Post an [issue](https://github.com/envkey/envkey-fetch/issues) or email us: [support@envkey.com](mailto:support@envkey.com).






