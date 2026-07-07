#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct ytdl_llama_runtime ytdl_llama_runtime;

typedef struct ytdl_llama_runtime_config {
    int context_tokens;
    int max_output_tokens;
    int threads;
    int gpu_layers;
    bool verbose_logging;
} ytdl_llama_runtime_config;

ytdl_llama_runtime * ytdl_llama_runtime_new(
    const char * model_path,
    const char * grammar_path,
    ytdl_llama_runtime_config config,
    char ** err_out);

int ytdl_llama_runtime_generate(
    ytdl_llama_runtime * runtime,
    const char * prompt,
    int max_tokens,
    float temperature,
    float top_p,
    char ** out_json,
    char ** err_out);

void ytdl_llama_runtime_cancel(ytdl_llama_runtime * runtime);
void ytdl_llama_runtime_free(ytdl_llama_runtime * runtime);
void ytdl_llama_string_free(char * value);

#ifdef __cplusplus
}
#endif
