# dev-keys/

**These keys are public. They are in a public git repository. Treat every one
of them as compromised, because it is.**

They exist so `garm-ai/agentd`'s `deploy/compose.yaml` comes up with one
command and behaves identically on every machine, and for no other reason. A
demo that made you generate keys first would be a demo most people never
finish, and one that generated them per run could not be checked into the
acceptance test.

| File | What it is | Who holds it |
|---|---|---|
| `sts-sign-k1.pem` | the STS's ES256 signing key | exported as `STS_SIGN_KEY_K1` by the `sts` service |
| `agentd.key` | agentd's `private_key_jwt` client key | `agentd --sts-client-key-file` |
| `agentd.pub.pem` | the public half, registered as client `agentd` | `clients:` in `deploy/sts/config.yaml` |

## What this means

Anyone reading this repository can sign a token that the demo's STS accepts as
`agentd`, and can forge tokens the demo's garmd accepts as anyone. That is
survivable here because the demo's STS trusts `garmdev idp`, which mints any
identity asked of it with no authentication at all — so the key material is
not what is holding anything up. Adding real keys to this arrangement would
protect nothing and would teach the wrong lesson about what does.

## Do not

- Do not point a staging or production STS at these keys, or at `config.yaml`
  next to them.
- Do not copy them into another repository "to get started".
- Do not add a real key here. Real key material is loaded from the environment
  and never referenced by path — `sts.LoadConfig` refuses an inline key
  outright, for exactly this reason.

Regenerate with the commands in `garm-ai/sts`'s `deploy/keygen.sh`; they are
ES256 on P-256 because the STS's algorithm allowlist accepts nothing else that
`openssl ecparam` produces by default.
