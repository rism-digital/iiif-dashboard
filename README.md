# IIIF Service Dashboard

IIIF Service Dashboard is a static website for comparing the behavior of public [IIIF](https://iiif.io/) implementations. It reports Presentation API and Image API support, content negotiation, CORS configuration, HTTP behavior, and Image API compliance levels using one curated sample from each project.

The dashboard is written in Elm. Network checks run separately in a Go command because browser code cannot inspect blocked cross-origin responses, duplicate headers, redirects, or complete `OPTIONS` responses. The resulting observations are stored in static JSON and displayed by the Elm application.

## What it checks

For a Presentation API sample manifest, the checker makes:

- a plain GET without an explicit `Accept` header;
- a GET requesting IIIF Presentation API 2;
- a GET requesting IIIF Presentation API 3; and
- an `OPTIONS` preflight for a GET with an `Accept` request header.

It identifies the returned IIIF version, checks for a recognizable manifest structure, verifies that the Manifest `id` (v3) or `@id` (v2) exactly matches the final response URL after redirects, reports the response media type, evaluates CORS, and checks negotiated responses for `Vary: Accept`.

For an Image API sample `info.json`, the checker makes:

- plain, Image API 2, and Image API 3 `info.json` requests;
- an `OPTIONS` preflight for `info.json`;
- a request to the image service base URI, checking for the recommended `303` redirect to `info.json`;
- a representative image request derived from `info.json`; and
- an `OPTIONS` preflight for that representative image.

It reports the detected Image API version, verifies that the Image Service `id` (v3) or `@id` (v2) matches the final `info.json` response URL with `/info.json` and any trailing slash removed, reports the declared Level 0, 1, or 2 compliance profile and representative image media type, and checks CORS behavior for both JSON and image responses.

The CORS diagnostics show every returned `Access-Control-*` header. A preflight passes when it returns exactly one usable `Access-Control-Allow-Origin`, permits `GET`, and permits the `Accept` request header. The configured dashboard origin or `*` is accepted.

The complete response-header set from each plain Presentation and Image API GET is also stored. It is available under the closed “Full response headers” disclosure in the diagnostic view.

## Status model

- **Pass:** the request and its primary diagnostic succeeded.
- **Warning:** the representation was usable but exposed an interoperability, content-negotiation, CORS, or HTTP issue.
- **Fail:** the request failed, returned an unusable representation, or did not have the expected IIIF structure. No response is a failure.
- **Not checked:** the project was intentionally omitted from a limited checker run.
- **Not tested:** the project has no sample for that API, no observation exists, or a prerequisite request failed.

The health meter counts each stored check once. The expanded CORS section presents another view of those observations and does not add duplicate blocks to the meter.

## Architecture

| Component | Location | Purpose |
| --- | --- | --- |
| Elm dashboard | `src/` | Loads the registry and latest snapshot and renders summaries and diagnostics. |
| Service checker | `cmd/iiif-checker/` | Makes concurrent HTTP requests and writes the observation snapshot. |
| Project registry | `projects.json` | Contributor-maintained list of projects and sample endpoints; copied into the built site. |
| Result snapshot | `dist/results.json` | Generated checker output consumed directly by the built dashboard. |
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
make check-services # Refresh dist/results.json from live services
make build-with-results # Build, run live checks, and produce a deployable site
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

Validate a registry edit before opening a pull request:

```sh
make validate
```

Pull-request validation is deliberately offline: it validates the JSON structure but does not contact contributed endpoints. Live requests are made by the scheduled checker after a contribution is merged.

## Running the service checker

Check every registered project and replace the current snapshot:

```sh
make check-services
```

The command can also be run directly:

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

Every checker request is unauthenticated and identifies itself with `IIIF-Checker-Bot/1.0 (https://rism-digital.github.io/iiif-dashboard) go-http-client/1.1`. Remember that `results.json` is public: the default-response header blocks may include cookies, CDN identifiers, and other metadata returned to anonymous clients.

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

`make build` creates the static Pages assets in `dist/`, copies `projects.json` there, and preserves an existing generated `dist/results.json`. If no results exist yet, it creates an empty snapshot so the dashboard remains usable. `make build-with-results` then runs the live checker and produces the complete deployable artifact.

Both deployment workflows use `make build-with-results`, so `results.json` is generated directly inside the Pages artifact and is not retained as a source file in the repository.

The weekly service workflow:

1. validates the registry;
2. builds the static site;
3. writes fresh observations directly to `dist/results.json`; and
4. deploys the refreshed static site.

For a custom domain, create a repository Actions variable named `DASHBOARD_ORIGIN` containing the production origin, for example:

```text
https://iiif.example.org
```

Use only the scheme and host, without a path or trailing slash. This ensures CORS is evaluated against the same origin from which users load the dashboard. In the repository’s Pages settings, select **GitHub Actions** as the publishing source and configure the custom domain there.

For the standard project site at `https://rism-digital.github.io/iiif-dashboard/`, the browser origin—and the checker's built-in default—is `https://rism-digital.github.io`. The `/iiif-dashboard/` path is not part of an HTTP origin.

## Scope and limitations

- Results are scheduled observations of curated samples, not continuous uptime monitoring.
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

Do not hand-edit `dist/results.json`; it is generated by the Go checker and excluded from version control with the rest of `dist/`.
