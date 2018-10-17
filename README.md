# Safe

`safe` is a command line tool for interacting with encrypted files. It provides a configuration file for tracking files and recipients.

`safe` uses `gpg` v1 to encrypt files. `safe` provides both a CLI and go library for managing and interacting with protected files

## Getting Started

In order to get started with `safe`, a `safe.yml` file must be created within a repository:

## Command Line Usage

### Create / Edit a file

To create a file or edit a previously encrypted file:

```bash
$ safe edit foo.md
```

### Protect a File

To encrypt and track a previously unencrypted file, `safe` provides `protect`:

```bash
$ safe protect foo.md
```

The `safe` CLI will add this file to it's list of tracked files, encrypt it and delete the original.

### Exec

`safe` provides a way to export secrets from a protected `yaml` file into an environment.

Given the `safe` protected file: `config.yml` with the contents:

```yaml
---
key: value
```

When `safe exec` is run with a command, the environment variable `KEY=value` will be exported:

```bash
$ safe exec config.yml.gpg.asc env | grep KEY
KEY=value
```

### Reencrypt a file

To reencrypt one or all tracked files with the current list of recipients, `safe` provides a `reencrypt` command.

```bash
$ safe reencrypt -all
```
