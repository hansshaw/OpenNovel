# Third-party notices

The application as a whole is AGPL-3.0-only. Third-party components keep their
original licenses; the retained license files are authoritative.

## Go dependencies

License and notice files copied unmodified from the exact versions in go.sum:

| Module | Version | Notices |
| --- | --- | --- |
| `github.com/davecgh/go-spew` | `v1.1.1` | [LICENSE](go/github.com__davecgh__go-spew@v1.1.1/LICENSE) |
| `github.com/gofrs/flock` | `v0.13.0` | [LICENSE](go/github.com__gofrs__flock@v0.13.0/LICENSE) |
| `github.com/pmezard/go-difflib` | `v1.0.0` | [LICENSE](go/github.com__pmezard__go-difflib@v1.0.0/LICENSE) |
| `github.com/stretchr/testify` | `v1.11.1` | [LICENSE](go/github.com__stretchr__testify@v1.11.1/LICENSE) |
| `github.com/voocel/agentcore` | `v1.8.3` | [LICENSE](go/github.com__voocel__agentcore@v1.8.3/LICENSE) |
| `github.com/voocel/litellm` | `v1.8.10` | [LICENSE](go/github.com__voocel__litellm@v1.8.10/LICENSE) |
| `golang.org/x/image` | `v0.45.0` | [LICENSE](go/golang.org__x__image@v0.45.0/LICENSE), [PATENTS](go/golang.org__x__image@v0.45.0/PATENTS) |
| `golang.org/x/mod` | `v0.40.0` | [LICENSE](go/golang.org__x__mod@v0.40.0/LICENSE), [PATENTS](go/golang.org__x__mod@v0.40.0/PATENTS) |
| `golang.org/x/net` | `v0.58.0` | [LICENSE](go/golang.org__x__net@v0.58.0/LICENSE), [PATENTS](go/golang.org__x__net@v0.58.0/PATENTS) |
| `golang.org/x/sys` | `v0.47.0` | [LICENSE](go/golang.org__x__sys@v0.47.0/LICENSE), [PATENTS](go/golang.org__x__sys@v0.47.0/PATENTS) |
| `golang.org/x/text` | `v0.41.0` | [LICENSE](go/golang.org__x__text@v0.41.0/LICENSE), [PATENTS](go/golang.org__x__text@v0.41.0/PATENTS) |
| `gopkg.in/yaml.v3` | `v3.0.1` | [NOTICE](go/gopkg.in__yaml.v3@v3.0.1/NOTICE), [LICENSE](go/gopkg.in__yaml.v3@v3.0.1/LICENSE) |

## Android bindings

The Android binding module pins its own Go tool dependencies in `mobile/bind/go.mod`
and `mobile/bind/go.sum`. The following additional versions retain their upstream
BSD license and patent notices. Go tool packages are build-time dependencies.

| Module | Version | Notices |
| --- | --- | --- |
| `golang.org/x/image` | `v0.47.0` | [LICENSE](go/golang.org__x__image@v0.47.0/LICENSE), [PATENTS](go/golang.org__x__image@v0.47.0/PATENTS) |
| `golang.org/x/mobile` | `v0.0.0-20261011034528-197770444867` | [LICENSE](go/golang.org__x__mobile@v0.0.0-20261011034528-197770444867/LICENSE), [PATENTS](go/golang.org__x__mobile@v0.0.0-20261011034528-197770444867/PATENTS) |
| `golang.org/x/mod` | `v0.42.0` | [LICENSE](go/golang.org__x__mod@v0.42.0/LICENSE), [PATENTS](go/golang.org__x__mod@v0.42.0/PATENTS) |
| `golang.org/x/net` | `v0.61.0` | [LICENSE](go/golang.org__x__net@v0.61.0/LICENSE), [PATENTS](go/golang.org__x__net@v0.61.0/PATENTS) |
| `golang.org/x/sync` | `v0.24.0` | [LICENSE](go/golang.org__x__sync@v0.24.0/LICENSE), [PATENTS](go/golang.org__x__sync@v0.24.0/PATENTS) |
| `golang.org/x/sys` | `v0.49.0` | [LICENSE](go/golang.org__x__sys@v0.49.0/LICENSE), [PATENTS](go/golang.org__x__sys@v0.49.0/PATENTS) |
| `golang.org/x/text` | `v0.43.0` | [LICENSE](go/golang.org__x__text@v0.43.0/LICENSE), [PATENTS](go/golang.org__x__text@v0.43.0/PATENTS) |
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

## Optional Breeze TTS service

The application includes an original HTTP adapter for the separately installed
[Breeze TTS 2 inference service](https://github.com/breezeblue-ai/breeze-tts).
No Breeze source, Python dependencies or model weights are bundled in this
repository or its application packages. The optional setup script downloads
upstream source commit `daa6cb2efe5471f09369229f322d20bb956cb3c3` into the user's
separate runtime directory, retaining its [Apache-2.0 license](https://github.com/breezeblue-ai/breeze-tts/blob/daa6cb2efe5471f09369229f322d20bb956cb3c3/LICENSE).

Model weights, derivative models and self-hosted outputs are governed separately
by the [BreezeBlue Research and Non-Commercial License](https://huggingface.co/BreezeBlue/breeze-tts-2/blob/main/LICENSE).
They are restricted to research and non-commercial use; commercial use requires
a separate written license from BreezeBlue. The application's AGPL license does
not grant rights to those model materials or outputs. See the
[setup guide](../docs/breeze-tts.md) for the integration and deployment scope.
