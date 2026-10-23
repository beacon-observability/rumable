# Rumable

Add browser real user monitoring (RUM) to websites through Nginx or IIS without changing application source code. Rumable rewrites HTML responses to load and initialize the bundled Web SDK. Browsers send events directly to the configured receiver; the CLI exits after deployment.

The compatible executable remains `rumable` / `rumable.exe`. Existing configuration fields, site IDs, state directories, resource URLs, and deployment markers retain their names so that updates and recovery continue to work. This documentation update does not establish a new public download channel or change the runtime data protocol.

IIS requires a started, dedicated application pool. Enable, update, disable, and rollback can recycle that pool and wait for old workers to exit. In-process sessions may be lost. Shared pools are rejected.

## Start here

| Task | Guide |
| --- | --- |
| First installation | [One-command setup](USER-GUIDE.md#3-one-command-setup) |
| Same-origin or CDN, asynchronous or synchronous loading | [SDK loading and updates](USER-GUIDE.md#4-sdk-loading-and-updates) |
| Update, disable, roll back, or uninstall | [Maintenance](USER-GUIDE.md#10-maintenance) |
| Look up TOML fields and defaults | [Configuration reference](docs/configuration.md) |
| Diagnose compression, caching, CSP, or drift | [Operations](docs/operations.md) |
| Check operating-system and server requirements | [Compatibility](docs/compatibility.md) |
| Configure optional server-side traces | [OTel guide](docs/otel.md) and [dependency installation](docs/otel-installation.md) |
| Maintain the configuration page and releases | [Page integration](docs/one-click-onboarding.md), [development](docs/development.md), and [release workflow](docs/gitlab-ci-release.md) |
| Review validation evidence | [Verification records](docs/verification.md) and [Windows acceptance](docs/windows-acceptance.md) |
| Recover a failed historical Windows deployment | [Recovery guide](docs/windows-0.1.4-recovery.md) |

## Choose SDK delivery

| Setting | Default | Alternative |
| --- | --- | --- |
| Source | Bundled SDK, served from the website origin | Package-default CDN source or a custom HTTPS CDN serving identical bytes |
| Loading | Asynchronous | Synchronous, which blocks HTML parsing while scripts load and execute |
| Update | Deploy the newer package to the existing site | Manage immutable CDN files and their cache policy |

Both servers support all four source/loading combinations. Asynchronous loading can miss events before initialization. The application version field and SDK remote configuration do not upgrade SDK files.

## Install and verify

1. Open the configuration page supplied with your release at `setup/index.html` over HTTP(S).
2. Enter application details, choose the server and SDK delivery, and load the matching release manifest.
3. Save the generated TOML and run the generated command on the actual Web server.
4. Check deployment status, open the website, inspect browser requests, and confirm stored events in the receiving application.

The page and `setup` command handle browser RUM. Server-side OTel is configured separately. Discovery configuration uses `server`, `[rum]`, and optional `[sdk]` with `setup --discover`; complete configuration uses `[[sites]]`, `[sites.rum]`, and optional `[sites.sdk]`.

For an existing site, keep its original configuration, deployment ID, and state directory:

~~~sh
rumable enable --config /etc/rumable/website.toml --state-dir /var/lib/rumable --site website --component rum
rumable status --state-dir /var/lib/rumable --site website
rumable probe --state-dir /var/lib/rumable --site website
~~~

Replacing the executable alone does not upgrade deployed SDK assets. Account for cached HTML during enable, disable, update, rollback, and uninstall. A successful HTTP probe proves injection and resource availability; browser execution and real ingestion require separate verification.

Try the [local Linux/Nginx demo](examples/local-demo/README.md) to inspect injection and requests to a local mock receiver.
