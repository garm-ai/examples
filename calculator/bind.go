package calculator

import (
	"context"

	"google.golang.org/protobuf/proto"

	calcv1 "github.com/garm-ai/examples/calculator/gen/calc/v1"
	"github.com/garm-ai/tool-go/toolbind"
)

// Register wires the handlers onto a runtime.
//
// Hand-written here, and it should not be: this is what
// protoc-gen-garm-go emits for a service, and the routes and request
// constructors below are exactly what a generator would derive from the
// descriptor. Written by hand so the runtime can be exercised before the
// tool-side generator is wired up, and it is the first thing to delete when
// it is.
func Register(r toolbind.Registrar, h Handlers) error {
	const svc = "calc.v1.Calculator"
	for _, m := range []toolbind.Method{
		{
			FullMethod: "/calc.v1.Calculator/Add",
			NewRequest: func() proto.Message { return &calcv1.AddRequest{} },
			Handle: func(ctx context.Context, req proto.Message) (proto.Message, error) {
				return h.Add(ctx, req.(*calcv1.AddRequest))
			},
		},
		{
			FullMethod: "/calc.v1.Calculator/Divide",
			NewRequest: func() proto.Message { return &calcv1.DivideRequest{} },
			Handle: func(ctx context.Context, req proto.Message) (proto.Message, error) {
				return h.Divide(ctx, req.(*calcv1.DivideRequest))
			},
		},
		{
			FullMethod: "/calc.v1.Calculator/Summarize",
			NewRequest: func() proto.Message { return &calcv1.SummarizeRequest{} },
			Handle: func(ctx context.Context, req proto.Message) (proto.Message, error) {
				return h.Summarize(ctx, req.(*calcv1.SummarizeRequest))
			},
		},
	} {
		if err := r.Register(svc, m); err != nil {
			return err
		}
	}
	return nil
}
