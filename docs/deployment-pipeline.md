# Deployment pipeline

Runnarr supports an optional repository-to-host delivery pipeline:

```text
trusted PR -> CI -> immutable GHCR image -> private synthetic preview
main       -> CI -> immutable GHCR image -> persistent staging
staging    -> manual GitHub approval     -> production
```

The normal self-hosted `docker compose up --build` workflow remains supported.
This runbook is for the single-host automated setup from Issue #179. It does
not authorize a production cutover by itself.

## Security and environment model

- CI for pull requests has no deployment, Cloudflare, provider, or host
  secrets.
- Only non-draft, same-repository PRs authored by users with write access can
  produce a host preview.
- The candidate image is built and scanned before GHCR credentials are used.
  Deployment jobs do not check out or execute PR scripts.
- Preview, staging, and production use different Compose projects, databases,
  volumes, state, credentials, and networks. Production's managed Compose
  project includes one persistent, resource-limited Valhalla graph.
- Each non-production app owns its unique ingress alias on its isolated
  network. The shared gateway joins that network without owning the alias.
- Automated previews can share one trusted, host-managed Valhalla container.
  The deployer attaches it independently to each preview network under the
  `valhalla` alias, verifies an actual route, and detaches it before teardown;
  preview application containers never share a common routing network.
- The Cloudflare Tunnel reaches the non-production gateway and the host SSH
  listener used by the restricted deployment account. It cannot reach
  PostgreSQL, production HTTP, the Docker socket, or other host stacks.
- Deployment SSH is not exposed by a router port-forward. Cloudflare Access
  service authentication reaches the tunnel, and the separate forced Ed25519
  key then authorizes only the matching deployment command set.
- Separate forced SSH keys allow either non-production commands or production
  commands. The production key is available only after GitHub Environment
  approval.
- Production accepts only the exact digest still recorded as the accepted
  staging deployment.

Containers are a strong application boundary, but previews still share the
host kernel with production. Automatic previews are therefore intentionally
limited to trusted same-repository writers.

## One-time host provisioning

Prerequisites:

- Docker Engine and Docker Compose;
- `jq`, `openssl`, and `age`;
- a Cloudflare-managed base domain and Cloudflare Tunnel;
- a private `ghcr.io/bznein/runnarr` package;
- an `age` recipient whose recovery identity is stored outside the deployment
  directory.

Run the installer from the reviewed repository revision:

```sh
sudo deploy/install-host.sh \
  example.com \
  00000000-0000-0000-0000-000000000000 \
  /root/cloudflare-tunnel-credentials.json
```

The installer creates:

- root-owned deployment assets under `/opt/runnarr-deploy`;
- configuration under `/etc/runnarr`;
- environment state and local backups under `/srv/runnarr`;
- a `runnarr-deploy` account in the Docker group;
- the external `runnarr-nonprod-ingress` network.

Membership in the Docker group is root-equivalent. Do not add an unrestricted
SSH key or shell automation for this account. Install two generated public keys
using [the forced-command example](../deploy/examples/authorized_keys.example).
Store their private halves only in the matching GitHub Environments.
The one-time `sudo deploy/install-deploy-keys.sh NONPROD_PUB PRODUCTION_PUB`
helper validates that the Ed25519 keys differ and installs those forced
commands without retaining key comments. It refuses to replace an existing
`authorized_keys` file.

Create the root-owned environment files:

```text
/srv/runnarr/config/preview.env
/srv/runnarr/environments/staging/base.env
/srv/runnarr/environments/production/base.env
/srv/runnarr/environments/production/image.env
```

Use owner/group `root:runnarr-deploy` and mode `0640` for `preview.env` and
both `base.env` files so the forced deployment account can read but not modify
them. `production/image.env` is managed by the deployment account and uses
owner/group `runnarr-deploy:runnarr-deploy` with mode `0600`.

Use the examples under `deploy/examples/`. The production `base.env` should be
a reviewed copy of the existing deployment `.env`; retain production OIDC,
database, provider, encryption, proxy, and resource settings. Do not copy those
values into preview or staging. Ensure bcrypt hashes containing `$` are
single-quoted in Compose environment files. Set `VALHALLA_TILE_URL` to an
HTTPS Geofabrik `.osm.pbf` extract that covers production users; the managed
production Compose path enables the bundled routing profile and points the app
at it on every promotion.

For initial staging setup, `sudo deploy/configure-staging.sh HASH_FILE`
validates a bare or formatted cost-12 bcrypt hash, generates independent database and
application secrets, leaves OIDC/provider credentials disabled, and installs
the root-owned `base.env`. It refuses to replace an existing staging file.
After adding the dedicated staging Google OIDC client and email mapping, run
`sudo deploy/verify-staging-oidc.sh` to validate the required values without
printing them.

Add these non-secret values to `/etc/runnarr/deploy.conf`:

```dotenv
RUNNARR_BACKUP_AGE_RECIPIENT=age1...
RUNNARR_STAGING_SEED_USERNAME=staging-admin
RUNNARR_PRODUCTION_URL=https://runnarr.example.com
```

Authenticate the deployment account to the private package with a classic PAT
that has only `read:packages`. The helper prompts without echoing the token,
streams it to Docker, and restricts the resulting Docker config:

```sh
sudo deploy/configure-ghcr-login.sh GITHUB_USERNAME
```

Do not run a broad Docker prune. The installer and cleanup jobs touch only
Runnarr-managed preview projects. Review existing old Runnarr E2E images and
build cache manually before enabling ten concurrent previews.

### Optional shared preview routing

The ordinary `routing` Compose profile is intended for one local stack. Do not
enable it separately in every automated preview because each Compose project
would build and retain another regional graph. Instead, run the reviewed
preview-routing helper once:

```sh
sudo deploy/configure-preview-routing.sh \
  https://download.geofabrik.de/europe/ireland-and-northern-ireland-latest.osm.pbf
```

The helper installs only the non-production overlay, restricted deployer, and
standalone routing Compose asset. It records the previous installed assets
under `/srv/runnarr/backups`, writes non-secret configuration to
`/etc/runnarr/preview-routing.env`, and starts `runnarr-nonprod-valhalla`
without restarting any existing Runnarr container. Wait for the graph build to
finish before rerunning previews. The image is pinned by digest, has no host
port, and receives explicit CPU, memory, and PID limits.

The helper defaults its routing smoke leg to central Dublin. When using a
different regional extract, provide four covered coordinates after the PBF URL
so preview acceptance exercises the selected graph.

When preview routing is active, newly deployed previews receive only the
internal `http://valhalla:8002` endpoint. The shared container is connected to
each isolated preview network and must pass a real Dublin pedestrian route
before that preview can be accepted. Staging remains unchanged.
Production uses its own bundled Valhalla graph; preview routing remains a
separate shared non-production service.

## Cloudflare configuration

Create exact DNS for staging:

```text
runnarr-staging.example.com CNAME <tunnel-id>.cfargotunnel.com
runnarr-deploy.example.com  CNAME <tunnel-id>.cfargotunnel.com
```

The preview workflow creates and removes exact
`runnarr-pr-<number>.example.com` CNAME records. Its Cloudflare token needs only
DNS edit access for the selected zone.

Create Access applications/policies for:

- `runnarr-staging.example.com`;
- `runnarr-pr-*.example.com`;
- `runnarr-deploy.example.com`.

The staging and preview applications should:

- deny by default and allow only reviewer identities;
- allow the dedicated service token used by deployment smoke tests;
- return `401` for invalid service authentication;
- use the same access policy without exposing the origin directly.

The deployment application should have a Service Auth policy that includes
the dedicated deployment service token. It routes to the host SSH listener
through `cloudflared`; do not forward TCP port 22 on the router. GitHub Actions
uses that token non-interactively, then authenticates SSH with the appropriate
forced Ed25519 key. Neither layer uses an account password.

On a host installed before the deployment SSH route was added, run
`sudo deploy/configure-tunnel-ssh.sh` before starting ingress. It backs up and
replaces only the installed ingress Compose and tunnel configuration, refuses
to run while ingress is active, and does not start containers.

Start the ingress after rendering and reviewing the generated configuration:

```sh
docker compose \
  --env-file /etc/runnarr/ingress/.env \
  -f /opt/runnarr-deploy/docker-compose.ingress.yml \
  up -d
```

Production continues to use its existing Nginx Proxy Manager route and proxy
network.

## GitHub configuration

Create four Environments:

| Environment | Required secrets | Variables |
| --- | --- | --- |
| `preview` | `DEPLOY_SSH_HOST`, `DEPLOY_SSH_USER`, `DEPLOY_SSH_KEY`, `DEPLOY_SSH_HOST_KEY`, `CF_DNS_API_TOKEN`, `CF_ACCESS_CLIENT_ID`, `CF_ACCESS_CLIENT_SECRET` | `DEPLOY_BASE_DOMAIN`, `CF_ZONE_ID`, `CF_TUNNEL_ID` |
| `staging` | `DEPLOY_SSH_HOST`, `DEPLOY_SSH_USER`, `DEPLOY_SSH_KEY`, `DEPLOY_SSH_HOST_KEY`, `CF_ACCESS_CLIENT_ID`, `CF_ACCESS_CLIENT_SECRET` | `DEPLOY_BASE_DOMAIN` |
| `production` | `DEPLOY_SSH_HOST`, `DEPLOY_SSH_USER`, `DEPLOY_SSH_KEY`, `DEPLOY_SSH_HOST_KEY`, `CF_ACCESS_CLIENT_ID`, `CF_ACCESS_CLIENT_SECRET` | `PRODUCTION_URL` |
| `visual-review-publish` | `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET` | None |

Use the non-production key in `preview` and `staging`, and the separate
production key in `production`. `DEPLOY_SSH_HOST_KEY` is the complete pinned
known-hosts line, not merely a fingerprint. Set `DEPLOY_SSH_HOST` in all three
environments to `runnarr-deploy.example.com` and `DEPLOY_SSH_USER` to
`runnarr-deploy`. The workflows download the tested native `cloudflared`
client with a pinned SHA-256 digest and use the Access service token without
storing it in the SSH command line.

Configure `production` with:

- `main` as the only deployment branch;
- at least one required reviewer;
- no wait timer;
- environment secrets unavailable until approval.

### Visual-review R2 media

The inline PR videos use a separate private Cloudflare R2 bucket. Provision it
manually; the repository deliberately does not own Cloudflare account or
billing configuration.

1. Enable R2 on the Cloudflare account and create a private Standard-storage
   bucket named `runnarr-visual-review`. Do not enable `r2.dev`, a custom public
   domain, Data Catalog, or Infrequent Access.
2. Add an object lifecycle rule for the `visual/` prefix that deletes objects
   after seven days. Newly uploaded objects must return an `x-amz-expiration`
   date no more than eight days away; the workflow deletes its uploads and
   fails if that cannot be verified.
3. Create an R2 API token restricted to Object Read & Write for only this
   bucket. Record its access key ID and secret once; do not reuse a deployment,
   DNS, or account-wide token.
4. Create the `visual-review-publish` GitHub Environment, restrict its
   deployment branch to `main`, and add the four secrets listed above. Set
   `R2_BUCKET=runnarr-visual-review`. Do not add a required reviewer because
   publishing is already gated before the environment job and is intended to
   finish automatically.
5. On a pay-as-you-go Cloudflare account, create an account-level **$1 budget
   alert** under Billing > Billable Usage and add the maintainer email. This is
   an informational alert, not a spending cap; inspect the daily R2 storage and
   Class A/Class B operation totals if it fires.

The code bounds accepted work to two profiles, four 60-second/25-MiB videos,
four 1-MiB posters, eight PUTs, and eight lifecycle HEAD requests per accepted
run. It uses fixed object keys under
`visual/pr-<number>/<head>/<revision>/`, so a rerun replaces rather than
accumulates the same revision. Objects stay private and the comment receives
only seven-day SigV4 GET URLs. Uploads are rolled back if validation, upload,
or lifecycle verification fails. GitHub ZIP artifacts remain the diagnostic
fallback and also expire after seven days.

R2 Standard currently includes monthly free allowances and free direct
egress, but that is not a hard guarantee of zero cost. Anyone who can read a
PR comment can reuse its signed GET URLs until they expire, so sufficiently
large repeated-read traffic can consume Class B operations. A Worker gateway
with per-link rate limits would add stronger read-abuse control, at the cost of
another deployed service, Worker request accounting, and more secrets; the
initial implementation intentionally avoids that operational surface. If the
budget alert fires unexpectedly, remove the visual labels, revoke the
bucket-scoped token (which invalidates its signed URLs), delete the affected
`visual/` objects, and remove the environment secrets before investigating.

Cloudflare references: [R2 pricing](https://developers.cloudflare.com/r2/pricing/),
[presigned URL limits](https://developers.cloudflare.com/r2/api/s3/presigned-urls/),
[object lifecycles](https://developers.cloudflare.com/r2/buckets/object-lifecycles/),
and [budget alerts](https://developers.cloudflare.com/billing/manage/budget-alerts/).

Protect `main` with pull requests and these required CI checks:

- Backend
- Frontend
- Browser E2E
- Docker Compose smoke
- Deployment configuration

Block force pushes and branch deletion. A mandatory second code reviewer is not
required for the current single-maintainer workflow.

## Lifecycle

### Pull-request preview

After CI succeeds, the default-branch-controlled candidate workflow builds,
scans, publishes, deploys, seeds, and smoke-tests the PR. It updates one PR
comment with the private URL, commit, digest, and deployment ID.

Every PR revision gets a fresh database and the deterministic E2E/testbed seed.
Provider credentials are absent. Closing the PR removes its stack, volumes,
network state, and DNS. Hourly reconciliation repairs missed cleanup.

When shared preview routing is configured, the generated preview environment
also enables Runnarr routing. Deployment fails closed if Valhalla is missing,
unhealthy, cannot join the isolated network, or cannot calculate the routing
smoke leg.

The initial limit is ten previews. Each app and PostgreSQL container is limited
to 0.5 CPU and 512 MiB. New previews are deferred below 12 GiB available memory
or 10 GiB free disk. Deployment networks use explicit non-overlapping subnets
so hosts with exhausted Docker default pools can still create them: production
retains `10.89.0.0/24`, ingress uses `10.90.0.0/24`, staging uses
`10.91.0.0/24`, shared preview routing uses `10.92.0.0/24`, and preview
subnets are deterministically allocated from the `10.100.0.0` through
`10.199.255.0` range.

### Staging

A successful `main` CI run automatically replaces staging with the new digest.
Its synthetic database persists so startup migrations are exercised across
deployments. Staging uses public-mode HTTPS/OIDC behavior plus a local
automation account behind Cloudflare Access.

If the migration-set label changes, the host starts the previous production
image briefly against the migrated synthetic staging database using staging's
complete base and generated image environment. Production promotion is blocked
unless that compatibility check succeeds.

Real provider accounts are disabled by default. Use only dedicated
non-production accounts during a separately declared integration smoke window.

### Production

Run `Production promotion` manually. The preflight job records the accepted
staging deployment before the production approval gate. If staging changes
while approval is pending, the promotion fails and must be restarted.

The host then:

1. verifies the image digest, commit, and migration identity;
2. verifies there are no running sync jobs;
3. checks backup disk capacity and stops the app for a brief maintenance
   window;
4. creates and validates encrypted PostgreSQL and `/app/data` snapshots;
5. starts the accepted digest without building;
6. starts or reuses the production Valhalla graph and waits for its health;
7. verifies the internal and external health commit;
8. records deployment and backup state. The promotion updates only
   `image.env`; the production routing overlay and named graph volume are
   preserved across image changes.

For the first managed promotion, the host preserves the existing local image
configuration before cutover. Older Runnarr images that expose only
`{"status":"ok"}` are accepted only for this bootstrap fallback. If the first
candidate fails, the exact local image configuration is restored and checked
using that legacy health response. Every managed image thereafter requires an
exact commit match.

Only the three newest verified local backups are retained, and pruning happens
only after a replacement backup and deployment succeed. These backups support
deployment rollback, but they do not protect against loss of the entire host.

If candidate startup fails, the host restores the previous compatible image.
The manually approved `Production image rollback` workflow can select only the
immediately previous deployment. Restoring a database/data snapshot remains a
manual break-glass operation: keep traffic stopped, verify the encrypted
snapshot and checksums, assess post-backup writes, and obtain explicit approval
before replacing production data.

## Migration contract

Production migrations remain forward-only and transactional. Every migration
must be additive and compatible with the previous production image for at
least one release. Destructive changes use expand/migrate/contract across
separate releases. A migration-changing PR must describe and test the previous
image compatibility path before merge.

## Troubleshooting and audit

The deploy-account-owned state files under `/srv/runnarr/environments/*/state.json`
record the digest, commit, migration hash, deployment time, backup, and previous
deployment. GitHub summaries record the same non-secret identity.

Useful read-only checks:

```sh
sudo -u runnarr-deploy \
  RUNNARR_DEPLOY_SCOPE=nonprod \
  /opt/runnarr-deploy/deploy/runnarr-deploy status staging

curl -fsS https://runnarr.example.com/healthz | jq
docker ps --filter label=com.runnarr.managed=true
```

Never print environment files, tunnel credentials, Access service secrets,
OIDC secrets, provider tokens, session cookies, or raw health data in Actions
or support logs.
