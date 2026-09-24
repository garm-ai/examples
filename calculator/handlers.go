// Package calculator implements the calculator tools.
//
// Handler bodies and nothing else. Authentication, authorisation, input
// checking and response redaction happened in the daemon before any of this
// ran, and repeating any of it here would be a second implementation of the
// chain in the one place that must not have one.
package calculator

import (
	"context"
	"errors"
	"math"
	"sort"

	calcv1 "github.com/garm-ai/examples/calculator/gen/calc/v1"
)

type Handlers struct{}

func (Handlers) Add(_ context.Context, r *calcv1.AddRequest) (*calcv1.AddResponse, error) {
	sum := r.GetA() + r.GetB()
	return &calcv1.AddResponse{Sum: &sum}, nil
}

func (Handlers) Divide(_ context.Context, r *calcv1.DivideRequest) (*calcv1.DivideResponse, error) {
	if r.GetDenominator() == 0 {
		// Refused rather than returned as ±Inf. A tool that answers +Inf has
		// told a model something it will carry into the next call, and the
		// guidance on this tool promises an error.
		return nil, errors.New("denominator is zero")
	}
	q := r.GetNumerator() / r.GetDenominator()
	return &calcv1.DivideResponse{Quotient: &q}, nil
}

func (Handlers) Summarize(_ context.Context, r *calcv1.SummarizeRequest) (*calcv1.SummarizeResponse, error) {
	vs := r.GetValues()
	if len(vs) == 0 {
		// An empty summary would be five zeroes, which reads as a real answer
		// about real data.
		return nil, errors.New("no values to summarize")
	}
	sorted := append([]float64(nil), vs...)
	sort.Float64s(sorted)

	var total float64
	for _, v := range sorted {
		total += v
	}
	mean := total / float64(len(sorted))

	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}

	min, max := sorted[0], sorted[len(sorted)-1]
	count := int64(len(sorted))
	if math.IsNaN(mean) {
		return nil, errors.New("values contain NaN")
	}
	return &calcv1.SummarizeResponse{
		Mean: &mean, Median: &median, Min: &min, Max: &max, Count: &count,
	}, nil
}
