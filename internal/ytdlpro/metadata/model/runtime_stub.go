//go:build !libllama

package model

import "fmt"

func newDefaultRuntime(cfg Config) (Runtime, error) {
	return nil, fmt.Errorf("%w: build without libllama tag", ErrRuntimeUnavailable)
}
