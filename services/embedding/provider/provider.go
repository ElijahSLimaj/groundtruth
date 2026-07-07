package provider

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
)

type Provider interface {
	ModelID() string
	Dimensions() int
	Embed(ctx context.Context, batch []string) ([][]float32, error)
}

type Fake struct {
	Model string
	Dims  int
	Fail  func(content string) error
}

func NewFake() *Fake {
	return &Fake{Model: "fake-embedder-v1", Dims: 1536}
}

func (f *Fake) ModelID() string {
	return f.Model
}

func (f *Fake) Dimensions() int {
	return f.Dims
}

func (f *Fake) Embed(_ context.Context, batch []string) ([][]float32, error) {
	vectors := make([][]float32, len(batch))
	for i, content := range batch {
		if f.Fail != nil {
			if err := f.Fail(content); err != nil {
				return nil, fmt.Errorf("embed item %d: %w", i, err)
			}
		}
		vectors[i] = deterministicVector(content, f.Dims)
	}
	return vectors, nil
}

func deterministicVector(content string, dims int) []float32 {
	seed := sha256.Sum256([]byte(content))
	vector := make([]float32, dims)
	var norm float64
	for i := range vector {
		var block [32]byte
		counter := sha256.Sum256(append(seed[:], byte(i), byte(i>>8)))
		copy(block[:], counter[:])
		bits := binary.BigEndian.Uint32(block[:4])
		v := float32(bits%2000)/1000 - 1
		vector[i] = v
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		vector[0] = 1
		return vector
	}
	for i := range vector {
		vector[i] = float32(float64(vector[i]) / norm)
	}
	return vector
}
