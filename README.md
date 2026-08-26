# IIIF Service Dashboard

IIIF Service Dashboard is a static website for comparing the behavior of public [IIIF](https://iiif.io/) implementations. It reports Presentation API and Image API support, content negotiation, CORS configuration, HTTP behavior, and Image API compliance levels using one curated sample from each project.

The dashboard is written in Elm. Network checks run separately in a Go command because browser code cannot inspect blocked cross-origin responses, duplicate headers, redirects, or complete `OPTIONS` responses. The resulting observations are stored in static JSON and displayed by the Elm application.

## What it checks

For a Presentation API sample manifest, the checker makes:

- a plain GET without an explicit `Accept` header;
- a GET requesting gzip compression;
- a GET requesting IIIF Presentation API 2;
- a GET requesting IIIF Presentation API 3; and
- an `OPTIONS` preflight for browser-based content negotiation with a profile-valued `Accept` request header.

It identifies the returned IIIF version, checks for a recognizable manifest structure, verifies that the Manifest `id` (v3) or `@id` (v2) exactly matches the final response URL after redirects, reports the response media type, tests gzip compression, evaluates CORS, and compares the v2 and v3 responses before deciding whether `Vary: Accept` is needed. When both requests return the same version, the matching request passes and the unavailable requested version is an advisory; `Vary: Accept` is required only when `Accept` observably changes the returned version or produces a success/406 distinction. A missing, empty, or non-string required identifier is a failure; a different non-empty identifier is a warning because the retrieved manifest is still recognizable and usable.

For an Image API sample `info.json`, the checker makes:

- plain, Image API 2, and Image API 3 `info.json` requests;
- an `info.json` request asking for gzip compression;
- an `OPTIONS` preflight for browser-based `info.json` content negotiation;
- a request to the image service base URI, checking for the recommended `303` redirect to `info.json`;
- a representative image request derived from `info.json`.

It reports the detected Image API version, verifies that the Image Service `id` (v3) or `@id` (v2) matches the final `info.json` response URL with `/info.json` and any trailing slash removed, tests gzip compression, reports the declared Level 0, 1, or 2 compliance profile and representative image media type, and checks CORS behavior for both JSON and image responses. Image API negotiation uses the same paired `Vary: Accept` policy as Presentation API negotiation. As with manifests, a missing, empty, or non-string required identifier fails, while a different non-empty identifier warns.

Dereferencing the image service base URI passes when it returns the IIIF-recommended `303` redirect to `info.json`. Any received non-`303` HTTP response is a warning rather than a failure because the redirect is recommended, not required. A request failure or a malformed `303` without a usable `Location` remains a failure.

The CORS diagnostics show every returned `Access-Control-*` header. Ordinary manifest, `info.json`, and image CORS support is evaluated from each actual GET response and does not require `Access-Control-Allow-Headers`. The separate manifest and `info.json` negotiation preflights pass when they return exactly one usable `Access-Control-Allow-Origin`, permit `GET`, and permit the `Accept` request header. Although `Accept` is normally CORS-safelisted, IIIF profile-valued forms contain quotes and a URI colon, which are CORS-unsafe characters and trigger a browser preflight when explicitly set. The configured dashboard origin or `*` is accepted.

The complete response-header set from each plain Presentation and Image API GET is also stored. When a request follows redirects, every intermediate status, `Location`, and complete response-header set is retained as a redirect chain. These are available under closed disclosures in the diagnostic view.

## Status model

- **Pass:** the request and its primary diagnostic succeeded.
- **Advisory:** the response is usable, but a recommended enhancement or best practice is absent.
- **Warning:** the representation was usable but exposed an interoperability, content-negotiation, CORS, or HTTP issue.
- **Fail:** the request failed, returned an unusable representation, or did not have the expected IIIF structure. No response is a failure.
- **Not checked:** the project was intentionally omitted from a limited checker run.
- **Not tested:** the project has no sample for that API, no observation exists, or a prerequisite request failed.

The main API badges and health meter summarize passes, warnings, and failures. Advisories do not downgrade those summaries; they appear as purple diagnostic blocks in the expanded details. The expanded CORS section presents another view of the stored observations and does not add duplicate blocks to the meter.

## Architecture

| Component | Location | Purpose |
| --- | --- | --- |
| Elm dashboard | `src/` | Loads the registry and latest snapshot and renders summaries and diagnostics. |
| Check documentation | `checks.html` | Publishes the requests, classification rules, possible outcomes, and references. |
| Service checker | `cmd/iiif-checker/` | Makes concurrent HTTP requests and writes the observation snapshot. |
| Project registry | `projects.json` | Contributor-maintained list of projects and sample endpoints; copied into the built site. |
| Result snapshot | `results.json` | Committed checker output; copied into the built dashboard. |
| Registry schema | `schema/projects.schema.json` | JSON Schema used for pull-request validation. |
| Build pipeline | `Makefile`, `optimize.sh`, `.swcrc` | Produces the static site with Elm and SWC. |
| Automation | `.github/workflows/` | Validates contributions, refreshes observations, and deploys GitHub Pages. |

The application has no runtime server or database. A production build consists only of HTML, CSS, JavaScript, and JSON files in `dist/`.

## Requirements

- Go 1.22 or newer
- Node.js 24 and Yarn 1
- GNU Make or a compatible `make`
- Python 3 for the local static development server

Elm 0.19.1, SWC, the Elm test runner, and the JSON Schema validator are installed through Yarn.

## Development

Install the JavaScript development dependencies:

```sh
yarn install --frozen-lockfile
```

Create a debug Elm build and serve it at `http://localhost:4173`:

```sh
make serve
```

Run the full local verification sequence:

```sh
make check
```

The individual targets are:

```sh
make validate       # Validate projects.json
make test           # Run Elm and Go tests
make build          # Build the optimized production site in dist/
make build-dev      # Build an unminified Elm debug site in dist/
make check-services # Refresh the committed results.json from live services
make build-with-results # Run live checks locally, then build the site
make clean
```

The production build runs `elm make --optimize`, minifies the result with SWC using Elm-specific compression settings, and removes the unminified intermediate. Vite and other JavaScript bundlers are not used.

## Project registry

The root-level `projects.json` is intended to be straightforward to find and edit in a pull request. Every project requires:

- a stable, kebab-case `id`;
- a human-readable `name`;
- an HTTPS `homepage`; and
- at least one of `manifestUrl` or `imageInfoUrl`.

Projects are kept in case-insensitive alphabetical order by `name`, with `id` used to break ties. Registry validation reports entries that are out of order, and the bundled import tools sort the registry when they write it.

A project may provide both API samples:

```json
{
  "id": "example-library",
  "name": "Example Library",
  "homepage": "https://www.example.org/",
  "manifestUrl": "https://iiif.example.org/presentation/manifest",
  "imageInfoUrl": "https://iiif.example.org/image/example/info.json"
}
```

Image-API-only projects are supported:

```json
{
  "id": "example-image-service",
  "name": "Example Image Service",
  "homepage": "https://images.example.org/",
  "imageInfoUrl": "https://images.example.org/iiif/example/info.json"
}
```

Likewise, `imageInfoUrl` may be omitted for a Presentation-API-only project. All sample URLs must be public HTTPS URLs, and an image sample must point to `info.json`.

Projects may also provide a public `notes` string for endpoint-specific context. The note is shown in the expanded dashboard details. In exceptional cases, `checkerUserAgent` may set a project-specific HTTP User-Agent when a service works in ordinary browsers but blocks the checker's identifying default. Overrides must be narrowly scoped and explained in `notes`; they are applied only to that project's requests and remain visible in the registry.

The checker recognizes legacy Image API 1.x contexts so that older, still-operational services receive accurate default-response diagnostics. Content-negotiation comparisons remain focused on Image API v2 and v3.

Validate a registry edit before opening a pull request:

```sh
make validate
```

Pull-request validation and deployment are deliberately offline: they do not contact contributed endpoints. Live requests are run locally and the resulting `results.json` snapshot is committed to the repository.

## Running the service checker

Check every registered project and replace the current snapshot:

```sh
make check-services
```

The command can also be run directly; it writes `results.json` by default:

```sh
go run ./cmd/iiif-checker
```

Useful options include:

| Option | Meaning |
| --- | --- |
| `-n 5` | Check the first five projects and mark the remainder “Not checked.” |
| `-project PROJECT-ID` | Refresh one project while preserving all other stored results. |
| `-concurrency 3` | Limit the number of projects checked concurrently. The default is six. |
| `-origin https://dashboard.example.org` | Set the origin used for CORS requests. |
| `-projects PATH` | Read a different project registry. |
| `-results PATH` | Write a different result snapshot. |

`-n 0` checks every project. `-n` and `-project` cannot be combined.

Interactive terminal runs display pending, active, and finished projects in a bounded in-place progress view. Redirected output and CI use stable log lines. A slow service occupies only its own worker and does not block unrelated projects.

Every checker request is unauthenticated and normally identifies itself with `IIIF-Checker-Bot/1.0 (https://rism-digital.github.io/iiif-dashboard) go-http-client/1.1`. A project-specific exception may be declared and explained publicly in `projects.json`, as described above. Remember that `results.json` is public: the default and intermediate redirect response headers may include cookies, CDN identifiers, and other metadata returned to anonymous clients.

## Registry maintenance tools

Several focused tools help curate sample endpoints.

### Import manifest hosts from CSV

```sh
scripts/import-manifest-services.sh
```

This reads `iiif_servers.csv`, selects one HTTPS manifest per previously unknown hostname, and appends registry entries named after their host. Optional input and registry paths may be supplied as the first and second arguments.

### Discover Image API samples

```sh
scripts/find-image-samples.sh --dry-run
scripts/find-image-samples.sh
```

The command traverses Presentation API 2 or 3 manifests, chooses candidate canvases, and stores a verified `info.json` URL. It never overwrites an existing image sample. Options include `--project`, `--seed`, `--concurrency`, and `--max-attempts`.

### Import from IIIF Universe

```sh
go run ./cmd/import-iiif-universe -dry-run
go run ./cmd/import-iiif-universe
```

The importer traverses IIIF Presentation API 2 and 3 collections from the IIIF Universe registry, skips hosts already represented locally, verifies a sample manifest, and attempts to find a corresponding Image API sample. Its traversal limits and random seed are configurable through command-line options.

### Resolve moved Harvard manifests

```sh
go run ./cmd/find-harvard-manifest-links
```

This single-purpose migration tool reads Harvard manifest URLs from `iiif_servers.csv`, extracts replacement links from their HTML responses, and writes `harvard_manifest_links.csv`. It supports alternate input and output paths and bounded concurrency.

## GitHub Pages deployment

`make build` creates the static Pages assets in `dist/`, including `checks.html`, and copies the committed `projects.json` and `results.json` files into the build. It never contacts external IIIF services. `make build-with-results` is a local convenience that refreshes `results.json` first and then builds the complete deployable artifact.

GitHub Actions deploys the committed snapshot with `make build`; checker execution in the deployment workflows is intentionally disabled. The former weekly refresh schedule is also disabled. To publish fresh observations:

1. run `make check-services` locally;
2. review the generated `results.json`, including any captured response headers;
3. commit `results.json`; and
4. push the commit so the normal Pages workflow builds and deploys it.

For a custom domain, set `DASHBOARD_ORIGIN` when running the checker locally, for example:

```sh
DASHBOARD_ORIGIN=https://iiif.example.org make check-services
```

Use only the scheme and host, without a path or trailing slash. This ensures CORS is evaluated against the same origin from which users load the dashboard. In the repository’s Pages settings, select **GitHub Actions** as the publishing source and configure the custom domain there.

For the standard project site at `https://rism-digital.github.io/iiif-dashboard/`, the browser origin—and the checker's built-in default—is `https://rism-digital.github.io`. The `/iiif-dashboard/` path is not part of an HTTP origin.

## Scope and limitations

- Results are point-in-time observations generated locally from curated samples, not continuous uptime monitoring.
- One manifest and one image service cannot demonstrate every capability of a large implementation.
- IIIF JSON checks recognize the relevant version and required top-level shape; they are not exhaustive conformance suites.
- The Image API level is the compliance profile declared by `info.json`; every feature required by that level is not exercised.
- Response headers are represented as deterministic header strings after Go’s HTTP parsing. They are not a byte-for-byte copy of the original wire order or casing.
- Redirect and cache behavior may vary by CDN edge, request time, and client location.

The browser-side prototype exposed several gaps in `elm-iiif`; they are recorded in [`GAPS.md`](GAPS.md). The production checker remains server-side so it can inspect HTTP behavior that browsers intentionally hide.

## Contributing

Contributions of new projects, corrected samples, checker improvements, and UI refinements are welcome. A typical contribution should:

1. edit `projects.json` or the relevant source code;
2. run `make validate` for registry changes;
3. run `make test`; and
4. open a pull request describing the service or behavior being added.

Do not hand-edit `results.json`; refresh it with `make check-services` and commit the generated changes. `dist/results.json` is only a build copy and remains excluded from version control with the rest of `dist/`.
