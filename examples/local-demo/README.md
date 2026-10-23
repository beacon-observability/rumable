# Local Rumable demo: Linux Web service and Nginx

Open [http://127.0.0.1:18874/](http://127.0.0.1:18874/). The demo is bound to loopback only.

Two Linux containers run a Node Web service and Nginx. The existing version-pinned 0.1.5 installer injects the SDK through Nginx; business HTML does not initialize it. Dummy application/token values, 100% session sampling, and replay disabled keep requests at the same-origin mock receiver. No real application ingestion is performed.

## Start

Requirements: Docker Compose, curl, and Node.js with native ESM, TextEncoder, and btoa. No CLI compilation inside the Linux containers is needed.

From the repository root:

~~~sh
bash examples/local-demo/start.sh
~~~

The script starts services, reads its pinned release manifest, invokes the real page generator, and runs its generated installation command. The first run downloads the published CLI/SDK package; later runs reuse versioned installation/state and the same site identity.

The demo's existing package download configuration is retained. This documentation does not create or claim a new Beacon-hosted package channel.

Open the home page, load products, make a slow request, trigger the expected error, or navigate to checkout. Filter Network by /v1/write/rum and wait for SDK batching. The receiver panel displays actual POST times, counts, bytes, and recognized event types. Unknown payloads count as requests without invented categories.

A mock 204 proves local receipt, not receiver authentication, real storage, or replay playback. The receiver does not validate credentials and is not production infrastructure. Requests are kept in Web-container memory, limited to 40 entries, and reset on restart. The intentional error button is expected to produce a Console error.

Generated TOML/commands, install logs, status, and probes are saved under ignored .local-demo/. These are not release objects.

## Inspect and disable

~~~sh
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml ps
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml exec -T nginx /opt/rumable/0.1.5/rumable status --state-dir /var/lib/rumable
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml logs --tail 20 web

# Stop injection on newly fetched pages; retain demo/state.
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml exec -T nginx /opt/rumable/0.1.5/rumable disable --state-dir /var/lib/rumable
~~~

Run start.sh again to re-enable using the saved identity. Compare raw backend HTML with Nginx output: only the filtered response has the managed data-rumable tag. The fixture does not inject it itself.

## Stop or remove

~~~sh
# Retain containers and state.
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml stop

# Remove only this demo's containers/network; retain named volumes.
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml down
~~~

Add --volumes only after confirming this demo's evidence/state is no longer required. Do not use global Docker pruning. Services use unless-stopped: an explicit stop prevents automatic restart.
