// The adopted proto packages, pinned by the only thing that pins a Go module.
//
// ../catalogue.yaml names `web.v1` from github.com/garm-ai/tools/web and
// `garm.artefacts.v1` from github.com/garm-ai/artefactd, and
// `garm catalogue build` resolves each out of the module cache at whatever
// version go.mod says. It refuses a module the tree does not require,
// deliberately: a manifest that could pin a version go.mod disagrees with would
// let this bank compile against one descriptor set and declare another.
//
// This tree calls neither module's Go. web.v1.fetch_page is invoked through the
// daemon over NATS by the research assistant, and the artefact store's four
// tools the same way by whatever generates a document; `webd` and `artefactd`,
// which do serve them, are somebody else's binaries. So without these lines
// `go mod tidy` drops the requirements and both manifest entries become
// illegal: the version of a contract this catalogue advertises would be pinned
// by nothing.
//
// A blank import is the ordinary Go idiom for a dependency you pin without
// calling, and it is what this file is: the requirements, written where a reader
// can see why they exist. The adopted taxonomy needs no line here because the
// generated code still carries one — a proto in this tree imports it, for the
// reason research_assistant.proto gives.
package bank

import (
	_ "github.com/garm-ai/artefactd/gen/garm/artefacts/v1"
	_ "github.com/garm-ai/tools/web/gen/web/v1"
)
