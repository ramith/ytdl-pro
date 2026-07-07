#include "llama_runtime.h"

#include <llama.h>

#include <exception>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

struct ytdl_llama_runtime {
    struct llama_model * model;
    struct llama_context * ctx;
    const struct llama_vocab * vocab;
    char * grammar;
    volatile int cancelled;
    ggml_log_callback prev_log_callback;
    void * prev_log_user_data;
    bool restore_logs;
};

static int ytdl_backend_refcount = 0;

static void ytdl_discard_log(enum ggml_log_level, const char *, void *) {
}

static bool ytdl_abort_callback(void * data) {
    struct ytdl_llama_runtime * runtime = (struct ytdl_llama_runtime *) data;
    return runtime != NULL && runtime->cancelled != 0;
}

static char * ytdl_strdup(const char * src) {
    if (src == NULL) {
        return NULL;
    }
    size_t len = strlen(src);
    char * out = (char *) malloc(len + 1);
    if (out == NULL) {
        return NULL;
    }
    memcpy(out, src, len + 1);
    return out;
}

static void ytdl_set_error(char ** err_out, const char * msg) {
    if (err_out == NULL) {
        return;
    }
    *err_out = ytdl_strdup(msg);
}

static char * ytdl_read_file(const char * path) {
    FILE * f = fopen(path, "rb");
    if (f == NULL) {
        return NULL;
    }
    if (fseek(f, 0, SEEK_END) != 0) {
        fclose(f);
        return NULL;
    }
    long size = ftell(f);
    if (size < 0) {
        fclose(f);
        return NULL;
    }
    if (fseek(f, 0, SEEK_SET) != 0) {
        fclose(f);
        return NULL;
    }
    char * data = (char *) malloc((size_t) size + 1);
    if (data == NULL) {
        fclose(f);
        return NULL;
    }
    size_t read_bytes = fread(data, 1, (size_t) size, f);
    fclose(f);
    if (read_bytes != (size_t) size) {
        free(data);
        return NULL;
    }
    data[size] = '\0';
    return data;
}

static int ytdl_tokenize(const struct llama_vocab * vocab, const char * text, llama_token ** out_tokens, int32_t * out_count, char ** err_out) {
    int32_t needed = llama_tokenize(vocab, text, (int32_t) strlen(text), NULL, 0, true, true);
    if (needed == INT32_MIN) {
        ytdl_set_error(err_out, "tokenization overflow");
        return -1;
    }
    if (needed < 0) {
        needed = -needed;
    }
    llama_token * tokens = (llama_token *) malloc(sizeof(llama_token) * (size_t) needed);
    if (tokens == NULL) {
        ytdl_set_error(err_out, "allocate tokens");
        return -1;
    }
    int32_t count = llama_tokenize(vocab, text, (int32_t) strlen(text), tokens, needed, true, true);
    if (count < 0) {
        free(tokens);
        ytdl_set_error(err_out, "tokenize prompt");
        return -1;
    }
    *out_tokens = tokens;
    *out_count = count;
    return 0;
}

static int ytdl_append_piece(const struct llama_vocab * vocab, llama_token token, char ** buffer, size_t * len, size_t * cap, char ** err_out) {
    char piece[256];
    int n = llama_token_to_piece(vocab, token, piece, (int32_t) sizeof(piece), 0, true);
    if (n < 0) {
        ytdl_set_error(err_out, "decode token piece");
        return -1;
    }
    if (*len + (size_t) n + 1 > *cap) {
        size_t next_cap = (*cap == 0) ? 1024 : *cap * 2;
        while (*len + (size_t) n + 1 > next_cap) {
            next_cap *= 2;
        }
        char * next = (char *) realloc(*buffer, next_cap);
        if (next == NULL) {
            ytdl_set_error(err_out, "grow output buffer");
            return -1;
        }
        *buffer = next;
        *cap = next_cap;
    }
    memcpy(*buffer + *len, piece, (size_t) n);
    *len += (size_t) n;
    (*buffer)[*len] = '\0';
    return 0;
}

static int ytdl_generate_with_sampler(
    ytdl_llama_runtime * runtime,
    const char * prompt,
    int max_tokens,
    float temperature,
    float top_p,
    bool use_grammar,
    char ** out_json,
    char ** err_out) {
    runtime->cancelled = 0;
    llama_memory_clear(llama_get_memory(runtime->ctx), true);

    llama_token * prompt_tokens = NULL;
    int32_t prompt_count = 0;
    if (ytdl_tokenize(runtime->vocab, prompt, &prompt_tokens, &prompt_count, err_out) != 0) {
        return -1;
    }

    struct llama_batch prompt_batch = llama_batch_get_one(prompt_tokens, prompt_count);
    int32_t decode_status = llama_decode(runtime->ctx, prompt_batch);
    free(prompt_tokens);
    if (decode_status != 0) {
        if (runtime->cancelled) {
            ytdl_set_error(err_out, "generation cancelled");
            return -2;
        }
        ytdl_set_error(err_out, "decode prompt");
        return -1;
    }

    struct llama_sampler * chain = llama_sampler_chain_init(llama_sampler_chain_default_params());
    if (chain == NULL) {
        ytdl_set_error(err_out, "create sampler chain");
        return -1;
    }

    if (use_grammar) {
        struct llama_sampler * grammar = llama_sampler_init_grammar(runtime->vocab, runtime->grammar, "root");
        if (grammar == NULL) {
            llama_sampler_free(chain);
            ytdl_set_error(err_out, "create grammar sampler");
            return -1;
        }
        llama_sampler_chain_add(chain, grammar);
    }

    if (temperature > 0.0f) {
        if (top_p > 0.0f && top_p < 1.0f) {
            llama_sampler_chain_add(chain, llama_sampler_init_top_p(top_p, 1));
        }
        llama_sampler_chain_add(chain, llama_sampler_init_temp(temperature));
        llama_sampler_chain_add(chain, llama_sampler_init_dist(7));
    } else {
        llama_sampler_chain_add(chain, llama_sampler_init_greedy());
    }

    char * output = NULL;
    size_t output_len = 0;
    size_t output_cap = 0;
    int status = 0;

    try {
        for (int i = 0; i < max_tokens; i++) {
            if (runtime->cancelled) {
                ytdl_set_error(err_out, "generation cancelled");
                status = -2;
                break;
            }

            llama_token token = llama_sampler_sample(chain, runtime->ctx, -1);
            if (llama_vocab_is_eog(runtime->vocab, token)) {
                break;
            }

            if (ytdl_append_piece(runtime->vocab, token, &output, &output_len, &output_cap, err_out) != 0) {
                status = -1;
                break;
            }

            llama_sampler_accept(chain, token);

            llama_token next_token = token;
            struct llama_batch next_batch = llama_batch_get_one(&next_token, 1);
            decode_status = llama_decode(runtime->ctx, next_batch);
            if (decode_status != 0) {
                if (runtime->cancelled) {
                    ytdl_set_error(err_out, "generation cancelled");
                    status = -2;
                    break;
                }
                ytdl_set_error(err_out, "decode generated token");
                status = -1;
                break;
            }
        }
    } catch (const std::exception & ex) {
        if (use_grammar) {
            ytdl_set_error(err_out, ex.what());
            status = -3;
        } else {
            ytdl_set_error(err_out, ex.what());
            status = -1;
        }
    } catch (...) {
        if (use_grammar) {
            ytdl_set_error(err_out, "grammar sampler failed");
            status = -3;
        } else {
            ytdl_set_error(err_out, "libllama generation failed");
            status = -1;
        }
    }

    llama_sampler_free(chain);

    if (status != 0) {
        free(output);
        return status;
    }

    if (output == NULL) {
        output = ytdl_strdup("");
    }
    *out_json = output;
    return 0;
}

ytdl_llama_runtime * ytdl_llama_runtime_new(
    const char * model_path,
    const char * grammar_path,
    ytdl_llama_runtime_config config,
    char ** err_out) {
    ggml_log_callback prev_log_callback = NULL;
    void * prev_log_user_data = NULL;
    bool quiet_logs = !config.verbose_logging;
    if (quiet_logs) {
        llama_log_get(&prev_log_callback, &prev_log_user_data);
        llama_log_set(ytdl_discard_log, NULL);
    }

    if (ytdl_backend_refcount == 0) {
        llama_backend_init();
    }
    ytdl_backend_refcount++;

    struct llama_model_params mparams = llama_model_default_params();
    mparams.n_gpu_layers = config.gpu_layers;
    mparams.use_mmap = true;

    struct llama_model * model = llama_model_load_from_file(model_path, mparams);
    if (model == NULL) {
        ytdl_set_error(err_out, "load GGUF model");
        ytdl_backend_refcount--;
        if (ytdl_backend_refcount == 0) {
            llama_backend_free();
        }
        if (quiet_logs) {
            llama_log_set(prev_log_callback, prev_log_user_data);
        }
        return NULL;
    }

    struct llama_context_params cparams = llama_context_default_params();
    cparams.n_ctx = (uint32_t) config.context_tokens;
    cparams.n_batch = (uint32_t) config.context_tokens;
    cparams.n_threads = config.threads;
    cparams.n_threads_batch = config.threads;

    struct llama_context * ctx = llama_init_from_model(model, cparams);
    if (ctx == NULL) {
        llama_model_free(model);
        ytdl_set_error(err_out, "create llama context");
        ytdl_backend_refcount--;
        if (ytdl_backend_refcount == 0) {
            llama_backend_free();
        }
        if (quiet_logs) {
            llama_log_set(prev_log_callback, prev_log_user_data);
        }
        return NULL;
    }

    struct ytdl_llama_runtime * runtime = (struct ytdl_llama_runtime *) calloc(1, sizeof(struct ytdl_llama_runtime));
    if (runtime == NULL) {
        llama_free(ctx);
        llama_model_free(model);
        ytdl_set_error(err_out, "allocate llama runtime");
        ytdl_backend_refcount--;
        if (ytdl_backend_refcount == 0) {
            llama_backend_free();
        }
        if (quiet_logs) {
            llama_log_set(prev_log_callback, prev_log_user_data);
        }
        return NULL;
    }

    runtime->grammar = ytdl_read_file(grammar_path);
    if (runtime->grammar == NULL) {
        free(runtime);
        llama_free(ctx);
        llama_model_free(model);
        ytdl_set_error(err_out, "load grammar file");
        ytdl_backend_refcount--;
        if (ytdl_backend_refcount == 0) {
            llama_backend_free();
        }
        if (quiet_logs) {
            llama_log_set(prev_log_callback, prev_log_user_data);
        }
        return NULL;
    }

    runtime->model = model;
    runtime->ctx = ctx;
    runtime->vocab = llama_model_get_vocab(model);
    runtime->cancelled = 0;
    runtime->prev_log_callback = prev_log_callback;
    runtime->prev_log_user_data = prev_log_user_data;
    runtime->restore_logs = quiet_logs;
    llama_set_abort_callback(ctx, ytdl_abort_callback, runtime);
    return runtime;
}

int ytdl_llama_runtime_generate(
    ytdl_llama_runtime * runtime,
    const char * prompt,
    int max_tokens,
    float temperature,
    float top_p,
    char ** out_json,
    char ** err_out) {
    if (runtime == NULL || prompt == NULL || out_json == NULL) {
        ytdl_set_error(err_out, "invalid llama generate arguments");
        return -1;
    }

    int status = ytdl_generate_with_sampler(runtime, prompt, max_tokens, temperature, top_p, true, out_json, err_out);
    if (status != -3) {
        return status;
    }

    if (err_out != NULL && *err_out != NULL) {
        free(*err_out);
        *err_out = NULL;
    }
    return ytdl_generate_with_sampler(runtime, prompt, max_tokens, temperature, top_p, false, out_json, err_out);
}

void ytdl_llama_runtime_cancel(ytdl_llama_runtime * runtime) {
    if (runtime != NULL) {
        runtime->cancelled = 1;
    }
}

void ytdl_llama_runtime_free(ytdl_llama_runtime * runtime) {
    if (runtime == NULL) {
        return;
    }
    free(runtime->grammar);
    if (runtime->ctx != NULL) {
        llama_free(runtime->ctx);
    }
    if (runtime->model != NULL) {
        llama_model_free(runtime->model);
    }
    if (runtime->restore_logs) {
        llama_log_set(runtime->prev_log_callback, runtime->prev_log_user_data);
    }
    free(runtime);
    ytdl_backend_refcount--;
    if (ytdl_backend_refcount == 0) {
        llama_backend_free();
    }
}

void ytdl_llama_string_free(char * value) {
    free(value);
}
