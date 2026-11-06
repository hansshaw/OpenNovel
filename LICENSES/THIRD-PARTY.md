# Third-party notices

The application as a whole is AGPL-3.0-only. Third-party components keep their
original licenses; the retained license files are authoritative.

## Go dependencies

License and notice files copied unmodified from the exact versions in go.sum,
including dependencies used by upstream tests:

| Module | Version | Notices |
| --- | --- | --- |
| `github.com/gofrs/flock` | `v0.13.1` | [LICENSE](go/github.com__gofrs__flock@v0.13.1/LICENSE) |
| `github.com/stretchr/testify` | `v1.12.1` | [LICENSE](go/github.com__stretchr__testify@v1.12.1/LICENSE), [internal/difflib/LICENSE](go/github.com__stretchr__testify@v1.12.1/internal/difflib/LICENSE), [internal/spew/LICENSE](go/github.com__stretchr__testify@v1.12.1/internal/spew/LICENSE) |
| `github.com/voocel/agentcore` | `v1.8.3` | [LICENSE](go/github.com__voocel__agentcore@v1.8.3/LICENSE) |
| `github.com/voocel/litellm` | `v1.8.11` | [LICENSE](go/github.com__voocel__litellm@v1.8.11/LICENSE) |
| `go.yaml.in/yaml/v3` | `v3.0.5` | [LICENSE](go/go.yaml.in__yaml__v3@v3.0.5/LICENSE), [NOTICE](go/go.yaml.in__yaml__v3@v3.0.5/NOTICE) |
| `golang.org/x/image` | `v0.47.0` | [LICENSE](go/golang.org__x__image@v0.47.0/LICENSE), [PATENTS](go/golang.org__x__image@v0.47.0/PATENTS) |
| `golang.org/x/mod` | `v0.41.0` | [LICENSE](go/golang.org__x__mod@v0.41.0/LICENSE), [PATENTS](go/golang.org__x__mod@v0.41.0/PATENTS) |
| `golang.org/x/net` | `v0.62.0` | [LICENSE](go/golang.org__x__net@v0.62.0/LICENSE), [PATENTS](go/golang.org__x__net@v0.62.0/PATENTS) |
| `golang.org/x/sys` | `v0.49.0` | [LICENSE](go/golang.org__x__sys@v0.49.0/LICENSE), [PATENTS](go/golang.org__x__sys@v0.49.0/PATENTS) |
| `golang.org/x/text` | `v0.43.0` | [LICENSE](go/golang.org__x__text@v0.43.0/LICENSE), [PATENTS](go/golang.org__x__text@v0.43.0/PATENTS) |

Testify includes copies of difflib and spew; their original BSD and ISC license
files are retained alongside Testify's MIT license. License directories for
previous dependency versions remain available for historical attribution.

## Android bindings

The Android binding module pins its own Go dependencies and tools in
`mobile/bind/go.mod` and `mobile/bind/go.sum`. Modules shared with the root module
use the notices above; additional module versions and test dependencies follow.
Go tool packages are build-time dependencies.

| Module | Version | Notices |
| --- | --- | --- |
| `github.com/google/go-cmp` | `v0.6.0` | [LICENSE](go/github.com__google__go-cmp@v0.6.0/LICENSE) |
| `golang.org/x/mobile` | `v0.0.0-20261011034528-197770444867` | [LICENSE](go/golang.org__x__mobile@v0.0.0-20261011034528-197770444867/LICENSE), [PATENTS](go/golang.org__x__mobile@v0.0.0-20261011034528-197770444867/PATENTS) |
| `golang.org/x/mod` | `v0.42.0` | [LICENSE](go/golang.org__x__mod@v0.42.0/LICENSE), [PATENTS](go/golang.org__x__mod@v0.42.0/PATENTS) |
| `golang.org/x/sync` | `v0.24.0` | [LICENSE](go/golang.org__x__sync@v0.24.0/LICENSE), [PATENTS](go/golang.org__x__sync@v0.24.0/PATENTS) |
| `golang.org/x/tools` | `v0.52.0` | [LICENSE](go/golang.org__x__tools@v0.52.0/LICENSE), [PATENTS](go/golang.org__x__tools@v0.52.0/PATENTS) |
| `golang.org/x/tools/go/expect` | `v0.1.1-deprecated` | [LICENSE](go/golang.org__x__tools__go__expect@v0.1.1-deprecated/LICENSE) |
| `golang.org/x/tools/go/packages/packagestest` | `v0.1.1-deprecated` | [LICENSE](go/golang.org__x__tools__go__packages__packagestest@v0.1.1-deprecated/LICENSE) |

Android uses the system WebView supplied by the device; Electron is not bundled
in the APK. Android SDK/NDK, Gradle and the JDK are build tools, not app payloads.

## PDF.js and browser runtime

PDF.js 6.4.299 is Apache-2.0; its font, CMap, ICC and codec notices remain
with the vendored assets under `internal/entry/desktopui/static/vendor/pdfjs/`.
An offline copy of those notices is also provided in [pdfjs/](pdfjs/LICENSE).
`desktop/scripts/sync-reader-vendor.js` verifies the pinned package and hashes.

Electron is MIT; Chromium and other runtime components retain their upstream
licenses. Electron's LICENSE and LICENSES.chromium.html are distributed by the
Electron packaging tooling with the runtime. npm build tools are development
dependencies; versions are fixed in desktop/package-lock.json.

The inherited source attribution and original Apache-2.0 license are preserved
in the root NOTICE and this directory. No third-party attribution is removed
by the repository's Git-history cleanup.
