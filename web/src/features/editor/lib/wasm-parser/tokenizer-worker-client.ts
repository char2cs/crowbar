import type {
  TokenizerWorkerRequest,
  TokenizerWorkerResponse,
  TokenizerWorkerResult,
  ViewportRangePayload,
} from './worker-protocol'

const POST_TIMEOUT_MS = 10_000

interface PendingRequest {
  resolve: (value: TokenizerWorkerResponse) => void
  reject: (reason?: unknown) => void
  timeoutHandle: ReturnType<typeof setTimeout>
}

class TokenizerWorkerClient {
  private worker: Worker | null = null
  private requestId = 0
  private pending = new Map<number, PendingRequest>()
  /** Maps bufferId → in-flight tokenize request id, so newer requests supersede older ones. */
  private pendingTokenizeByBuffer = new Map<string, number>()
  private readonly timeoutMs: number

  constructor(timeoutMs = POST_TIMEOUT_MS) {
    this.timeoutMs = timeoutMs
  }

  private ensureWorker(): Worker {
    if (this.worker) return this.worker

    this.worker = new Worker(new URL('./tokenizer-worker.ts', import.meta.url), {
      type: 'module',
    })

    this.worker.onmessage = (event: MessageEvent<TokenizerWorkerResponse>) => {
      const message = event.data
      const pending = this.pending.get(message.id)
      if (!pending) return
      clearTimeout(pending.timeoutHandle)
      this.pending.delete(message.id)

      if (message.ok) {
        pending.resolve(message)
      } else {
        pending.reject(new Error(message.error))
      }
    }

    this.worker.onerror = (event) => {
      const error = event.error || new Error(event.message)
      for (const pending of this.pending.values()) {
        clearTimeout(pending.timeoutHandle)
        pending.reject(error)
      }
      this.pending.clear()
      this.pendingTokenizeByBuffer.clear()
    }

    return this.worker
  }

  private post<T extends TokenizerWorkerResponse>(
    request: TokenizerWorkerRequest,
    bufferId?: string,
  ): Promise<T> {
    const worker = this.ensureWorker()

    // Supersede any existing in-flight tokenize request for the same buffer.
    if (bufferId !== undefined) {
      const priorId = this.pendingTokenizeByBuffer.get(bufferId)
      if (priorId !== undefined) {
        const prior = this.pending.get(priorId)
        if (prior) {
          clearTimeout(prior.timeoutHandle)
          prior.reject(new Error('tokenizer request superseded by newer request for same buffer'))
          this.pending.delete(priorId)
        }
        this.pendingTokenizeByBuffer.delete(bufferId)
      }
    }

    return new Promise<T>((resolve, reject) => {
      const timeoutHandle = setTimeout(() => {
        if (this.pending.has(request.id)) {
          this.pending.delete(request.id)
          if (bufferId !== undefined && this.pendingTokenizeByBuffer.get(bufferId) === request.id) {
            this.pendingTokenizeByBuffer.delete(bufferId)
          }
          reject(new Error('tokenizer worker timed out'))
        }
      }, this.timeoutMs)

      this.pending.set(request.id, {
        resolve: resolve as (value: TokenizerWorkerResponse) => void,
        reject,
        timeoutHandle,
      })

      if (bufferId !== undefined) {
        this.pendingTokenizeByBuffer.set(bufferId, request.id)
      }

      worker.postMessage(request)
    })
  }

  async warmup(languages?: string[]): Promise<void> {
    const id = ++this.requestId
    await this.post({ id, type: 'warmup', languages })
  }

  async reset(bufferId: string): Promise<void> {
    const id = ++this.requestId
    await this.post({ id, type: 'reset', bufferId })
  }

  async tokenize(params: {
    bufferId: string
    content: string
    languageId: string
    wasmPath?: string
    highlightQueryUrl?: string
    mode: 'full' | 'range'
    viewportRange?: ViewportRangePayload
  }): Promise<TokenizerWorkerResult> {
    const id = ++this.requestId
    const response = await this.post<Extract<TokenizerWorkerResponse, { ok: true }>>(
      {
        id,
        type: 'tokenize',
        ...params,
      },
      params.bufferId,
    )

    return {
      tokens: response.tokens ?? [],
      normalizedText: response.normalizedText ?? params.content,
    }
  }
}

export { TokenizerWorkerClient }
export const tokenizerWorkerClient = new TokenizerWorkerClient()
