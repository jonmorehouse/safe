# Safe

A command line tool for managing secrets with `git`

## Note

This isn't real software yet; keep tabs over at: github.com/jonmorehouse/safe/issues


## CLI Overview

- get: print a single key from a secrets file
- edit: edit a secret file
- decrypt: decrypt a file to either stdout or another file
- join: add yourself to the keyring, asking to be accepted
- accept: accept a user into the keyring, re-encrypting all files
- kick: remove someone from the keyring
- create-keyring: create a new keyring
- create: create a new encrypted file
- destroy-keyring: destroy an entire keyring
- destroy: destroys a single file
