# AGENTS.md

This file provides guidance to AI coding agents and contributors working in this part of the repository.

Scope: this file covers the `infra/` directory only (Terraform for the Hetzner
server and object storage). See the repository root [AGENTS.md](../AGENTS.md)
for cross-component rules. Setup order, required GitHub secrets/variables,
environments, hardening and backup notes live in [README.md](README.md) —
this file does not repeat them; update that README when they change.

## Layout

- `bootstrap/` — run **once, locally**, creates the private state and backup
  buckets in Hetzner Object Storage. Its state is local and not in the repo.
- `hetzner/` — server, firewall, volume for the PostgreSQL data, cloud-init.
  State lives in the bucket from `bootstrap/`. Rolled out by
  `.github/workflows/infra.yml` (`apply` behind the GitHub environment `infra`).

## Commands

```bash
cd infra/hetzner
terraform fmt -recursive              # CI runs `terraform fmt -check -recursive`
terraform init -backend=false         # enough for validate, no credentials needed
terraform validate
```

`plan`/`apply` need the remote state backend and `HCLOUD_TOKEN`; they are run
by `infra.yml`, not by hand. The application itself is deployed by
`.github/workflows/main.yml`, not by Terraform.

## Rules

- **Never commit secrets or state** (`*.tfstate`, `backend.hcl`, tokens, SSH
  keys, S3 keys). Credentials come from GitHub secrets/variables (table in
  [README.md](README.md#github-secrets-und-variablen)); add a new one there
  and wire it in the workflow rather than hard-coding it.
- The PostgreSQL volume has `prevent_destroy`; do not remove it. Rebuild the
  server with `terraform taint hcloud_server.main` (the data stays on the volume).
- `cloud-init` changes only affect newly created servers (`user_data` is in
  `ignore_changes`).
- Third-party GitHub Actions are pinned by commit SHA (Dependabot keeps them
  current); keep that when adding steps.
