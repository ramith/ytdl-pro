//go:build libllama && (darwin || linux)

package llama

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo CXXFLAGS: -std=c++17 -I${SRCDIR}
#cgo darwin CFLAGS: -I/opt/homebrew/include -I/opt/homebrew/opt/llama.cpp/include -I/usr/local/include
#cgo darwin CXXFLAGS: -I/opt/homebrew/include -I/opt/homebrew/opt/llama.cpp/include -I/usr/local/include
#cgo linux CFLAGS: -I/usr/local/include -I/usr/include
#cgo linux CXXFLAGS: -I/usr/local/include -I/usr/include
#cgo darwin LDFLAGS: -L${SRCDIR}/../../../../../lib -L/opt/homebrew/lib -L/usr/local/lib -lllama -Wl,-rpath,${SRCDIR}/../../../../../lib
#cgo linux LDFLAGS: -L${SRCDIR}/../../../../../lib -L/usr/local/lib -L/usr/lib -lllama -Wl,-rpath,${SRCDIR}/../../../../../lib -ldl -lm -lstdc++
#include <stdlib.h>
#include "llama_runtime.h"
*/
import "C"

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"ytdl-pro/internal/ytdlpro/metadata/model/spec"
)

type Config struct {
	Model           string
	ModelPath       string
	GrammarPath     string
	ContextTokens   int
	MaxOutputTokens int
	Threads         string
	GPULayers       string
	Debug           bool
	Explain         bool
}

type Runtime struct {
	state  *C.ytdl_llama_runtime
	reqs   chan generateRequest
	wg     sync.WaitGroup
	closed chan struct{}
	once   sync.Once
}

type generateRequest struct {
	ctx    context.Context
	prompt string
	opts   spec.GenerateOptions
	resp   chan generateResponse
}

type generateResponse struct {
	text string
	err  error
}

func NewRuntime(cfg Config) (*Runtime, error) {
	threads, err := parseMaybeAutoInt(cfg.Threads, 0)
	if err != nil {
		return nil, fmt.Errorf("parse metadata threads: %w", err)
	}
	gpuLayers, err := parseMaybeAutoInt(cfg.GPULayers, -1)
	if err != nil {
		return nil, fmt.Errorf("parse metadata gpu layers: %w", err)
	}

	cModelPath := C.CString(cfg.ModelPath)
	defer C.free(unsafe.Pointer(cModelPath))
	cGrammarPath := C.CString(cfg.GrammarPath)
	defer C.free(unsafe.Pointer(cGrammarPath))

	var cErr *C.char
	state := C.ytdl_llama_runtime_new(
		cModelPath,
		cGrammarPath,
		C.ytdl_llama_runtime_config{
			context_tokens:    C.int(cfg.ContextTokens),
			max_output_tokens: C.int(cfg.MaxOutputTokens),
			threads:           C.int(threads),
			gpu_layers:        C.int(gpuLayers),
			verbose_logging:   C.bool(cfg.Debug),
		},
		&cErr,
	)
	if state == nil {
		defer freeCString(cErr)
		return nil, runtimeError(cErr)
	}

	runtime := &Runtime{
		state:  state,
		reqs:   make(chan generateRequest),
		closed: make(chan struct{}),
	}
	runtime.wg.Add(1)
	go runtime.worker()
	return runtime, nil
}

func (r *Runtime) GenerateJSON(ctx context.Context, prompt string, opts spec.GenerateOptions) (string, error) {
	if r == nil {
		return "", fmt.Errorf("libllama runtime is nil")
	}

	req := generateRequest{
		ctx:    ctx,
		prompt: prompt,
		opts:   opts,
		resp:   make(chan generateResponse, 1),
	}

	select {
	case <-r.closed:
		return "", fmt.Errorf("libllama runtime is closed")
	case r.reqs <- req:
	}

	select {
	case <-ctx.Done():
		C.ytdl_llama_runtime_cancel(r.state)
		return "", ctx.Err()
	case resp := <-req.resp:
		return resp.text, resp.err
	}
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.once.Do(func() {
		close(r.closed)
		close(r.reqs)
		r.wg.Wait()
		if r.state != nil {
			C.ytdl_llama_runtime_free(r.state)
			r.state = nil
		}
	})
	return nil
}

func (r *Runtime) worker() {
	defer r.wg.Done()
	for req := range r.reqs {
		text, err := r.generate(req.prompt, req.opts)
		req.resp <- generateResponse{text: text, err: err}
	}
}

func (r *Runtime) generate(prompt string, opts spec.GenerateOptions) (string, error) {
	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))

	var cOut *C.char
	var cErr *C.char
	status := C.ytdl_llama_runtime_generate(
		r.state,
		cPrompt,
		C.int(opts.MaxTokens),
		C.float(opts.Temperature),
		C.float(opts.TopP),
		&cOut,
		&cErr,
	)
	defer freeCString(cOut)
	defer freeCString(cErr)
	if status != 0 {
		return "", runtimeError(cErr)
	}
	return strings.TrimSpace(C.GoString(cOut)), nil
}

func runtimeError(cErr *C.char) error {
	if cErr == nil {
		return fmt.Errorf("libllama runtime error")
	}
	return fmt.Errorf("%s", C.GoString(cErr))
}

func freeCString(value *C.char) {
	if value != nil {
		C.ytdl_llama_string_free(value)
	}
}

func parseMaybeAutoInt(raw string, autoValue int) (int, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "", "auto":
		return autoValue, nil
	}
	var parsed int
	_, err := fmt.Sscanf(value, "%d", &parsed)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value %q", raw)
	}
	return parsed, nil
}
