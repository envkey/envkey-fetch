# envkey-fetch

This library contains [EnvKey](https://www.envkey.com)'s core cross-platform fetching, decryption, verification, web of trust, redundancy, and caching logic. It accepts an `ENVKEY` and returns decrypted configuration for a specific app environment as json.

It is used by EnvKey's various Client Libraries, including [envkey-source](https://github.com/envkey/envkey-source) for bash, [envkey-ruby](https://github.com/envkey/envkey-ruby) for Ruby and Rails, [envkey-js](https://github.com/envkey/envkey-js) for Node.js, and [envkeygo](https://github.com/envkey/envkeygo) for Go.

If you want to build an EnvKey library in a language that isn't yet officially supported, build some other type of integration, or simply play around with EnvKey on the command line, envkey-fetch is the library for you. If you just want to integrate EnvKey with your project, check out one of the aforementioned higher level libraries.

## Installation

envkey-fetch is a simple static binary with no dependencies.

**Via bash:**

```bash
curl -s https://raw.githubusercontent.com/envkey/envkey-fetch/master/install.sh | bash
```

**Manually:**

Find the [release](https://github.com/envkey/envkey-fetch/releases) for your platform and architecture, and stick the appropriate binary somewhere in your `PATH` (or wherever you like really).

**From source:**

With Go installed, clone the project into your `GOPATH`. Run `go get` and `go build`.

## Usage

```bash
envkey-fetch YOUR-ENVKEY [flags]
```

## Flags

```bash
--cache              cache encrypted config as a local backup (default is false)
--cache-dir string   cache directory (default is $HOME/.envkey/cache)
-h, --help           help for envkey-fetch
-v, --version        prints the version
```

## Further Reading

For more on EnvKey in general:

Read the [docs](https://docs.envkey.com).

Read the [integration quickstart](https://docs.envkey.com/integration-quickstart.html).

Read the [security and cryptography overview](https://security.envkey.com).

## Need help? Have questions, feedback, or ideas?

Post an [issue](https://github.com/envkey/envkey-fetch/issues) or email us: [support@envkey.com](mailto:support@envkey.com).






