// The adopted proto package, pinned by the only thing that pins a Go module.
//
// ../catalogue.yaml names `web.v1` from github.com/garm-ai/tools/web, and
// `garm catalogue build` resolves it out of the module cache at whatever
// version go.mod says. It refuses a module the tree does not require,
// deliberately: a manifest that could pin a version go.mod disagrees with would
// let this bank compile against one descriptor set and declare another.
//
// This tree calls none of that module's Go. web.v1.fetch_page is invoked
// through the daemon over NATS by the research assistant, never in process, and
// `webd` — which does serve it — is somebody else's binary. So without this
// line `go mod tidy` drops the requirement and the manifest entry becomes
// illegal: the version of a contract this catalogue advertises would be pinned
// by nothing.
//
// A blank import is the ordinary Go idiom for a dependency you pin without
// calling, and it is what this file is: the requirement, written where a reader
// can see why it exists. The adopted taxonomy needs no line here because the
// generated code still carries one — a proto in this tree imports it, for the
// reason research_assistant.proto gives.
package bank

import _ "github.com/garm-ai/tools/web/gen/web/v1"
