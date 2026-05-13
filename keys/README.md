# Release signing keys

This directory holds the GPG public keys used to sign release artifacts.
The matching private keys live outside the repository and are loaded
into the release workflow via GitHub Actions secrets.

Anyone verifying a downloaded release can:

```bash
gpg --import keys/release-signing.asc
gpg --verify terraform-provider-datadog-feature-flags_<version>_SHA256SUMS.sig \
              terraform-provider-datadog-feature-flags_<version>_SHA256SUMS
```

The Terraform Registry verifies the same signature automatically when it
indexes a new release; if a future release is signed with a different
key, that key must be added to the Registry's "Signing Keys" page
*and* committed here.

## Key inventory

| File | Fingerprint | Use |
| --- | --- | --- |
| `release-signing.asc` | `35A2 90BF 6313 F765 8645 95F9 666C 4767 3D0F BDEE` | Primary release signing key for v0.1.x and later. |
