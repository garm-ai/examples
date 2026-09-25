package calculator_test

import (
	"fmt"
	"testing"

	calcv1 "github.com/garm-ai/examples/calculator/gen/calc/v1"
	"github.com/garm-ai/tool-go/garmtool"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// What a tool call costs, measured.
//
// This is the HOP ONLY: marshal, NATS request/reply, unmarshal, handler. The
// governance chain is not in this path yet, so these are a floor rather than
// an answer — see the repository README for what the chain adds.
//
//	go test ./calculator/ -bench=. -benchtime=2s -run=XXX

func benchConn(b *testing.B) *nats.Conn {
	b.Helper()
	// Reuses the same embedded server and service the e2e test builds, via a
	// testing.TB so one helper serves both.
	nc := runNATSB(b)
	serveB(b, nc)
	return nc
}

func BenchmarkCallAdd(b *testing.B) {
	nc := benchConn(b)
	a, bb := 2.5, 4.0
	req, _ := proto.Marshal(&calcv1.AddRequest{A: &a, B: &bb})
	subject := garmtool.Subject("/calc.v1.Calculator/Add")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg, err := nc.Request(subject, req, benchTimeout)
		if err != nil {
			b.Fatal(err)
		}
		var resp calcv1.AddResponse
		if err := proto.Unmarshal(msg.Data, &resp); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCallSummarize scales the PAYLOAD rather than the tool count.
// Marshalling and unmarshalling are linear in message size, and this is where
// that shows up.
func BenchmarkCallSummarize(b *testing.B) {
	nc := benchConn(b)
	subject := garmtool.Subject("/calc.v1.Calculator/Summarize")

	for _, n := range []int{10, 100, 1000, 10000} {
		b.Run(fmt.Sprintf("%dvalues", n), func(b *testing.B) {
			vs := make([]float64, n)
			for i := range vs {
				vs[i] = float64(i)
			}
			req, _ := proto.Marshal(&calcv1.SummarizeRequest{Values: vs})
			b.SetBytes(int64(len(req)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				msg, err := nc.Request(subject, req, benchTimeout)
				if err != nil {
					b.Fatal(err)
				}
				var resp calcv1.SummarizeResponse
				if err := proto.Unmarshal(msg.Data, &resp); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCallParallel is throughput rather than latency. A tool plane
// serves many agents at once, and the interesting question is whether one
// slow call blocks others — it should not, since every request is its own
// goroutine on both sides.
func BenchmarkCallParallel(b *testing.B) {
	nc := benchConn(b)
	a, bb := 2.5, 4.0
	req, _ := proto.Marshal(&calcv1.AddRequest{A: &a, B: &bb})
	subject := garmtool.Subject("/calc.v1.Calculator/Add")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := nc.Request(subject, req, benchTimeout); err != nil {
				b.Fatal(err)
			}
		}
	})
}
