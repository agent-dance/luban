#!/usr/bin/env node
const fs = require('fs')
const path = require('path')

function emit(obj) {
  process.stdout.write(JSON.stringify(obj, null, 2))
}

function webFetchContract(input) {
  const out = {
    tool: 'WebFetch',
    input: {
      url: input.url || '',
      prompt: input.prompt || '',
    },
    validation: {
      ok: true,
      message: '',
      errorCode: 0,
    },
    semantics: {
      providerNative: true,
      promptDrivenExtraction: true,
      hasDedicatedPrompt: true,
      permissionAware: true,
      resultShape: {
        bytes: 'number',
        code: 'number',
        codeText: 'string',
        result: 'string',
        durationMs: 'number',
        url: 'string',
      },
    },
    normalizedResult: null,
  }

  try {
    new URL(out.input.url)
  } catch {
    out.validation = {
      ok: false,
      message: `Error: Invalid URL "${out.input.url}". The URL provided could not be parsed.`,
      errorCode: 1,
    }
    out.normalizedResult = {
      input: { url: out.input.url, prompt: out.input.prompt },
      execution: { method: 'provider_native', resolvedUrl: out.input.url, truncated: false, cacheHit: false },
      content: { body: '' },
      error: out.validation.message,
    }
    return out
  }

  const body = typeof input.mock_result === 'string' ? input.mock_result : `Fetched content for: ${out.input.prompt}`
  out.normalizedResult = {
    input: { url: out.input.url, prompt: out.input.prompt },
    execution: { method: 'provider_native', resolvedUrl: out.input.url, truncated: false, cacheHit: false },
    content: { body },
  }

  return out
}

function webSearchContract(input) {
  const out = {
    tool: 'WebSearch',
    input: {
      query: input.query || '',
      allowed_domains: Array.isArray(input.allowed_domains) ? input.allowed_domains : [],
      blocked_domains: Array.isArray(input.blocked_domains) ? input.blocked_domains : [],
    },
    validation: {
      ok: true,
      message: '',
      errorCode: 0,
    },
    semantics: {
      providerNative: true,
      promptDrivenExtraction: true,
      hasDedicatedPrompt: true,
      permissionAware: true,
      progressEvents: ['query_update', 'search_results_received'],
      resultShape: {
        query: 'string',
        results: 'array<string|search_result>',
        durationSeconds: 'number',
      },
    },
    normalizedResult: null,
  }

  if (!out.input.query.length) {
    out.validation = {
      ok: false,
      message: 'Error: Missing query',
      errorCode: 1,
    }
    out.normalizedResult = {
      input: { query: out.input.query, allowedDomains: out.input.allowed_domains, blockedDomains: out.input.blocked_domains },
      execution: { method: 'provider_native', fallbackReason: '', cacheHit: false },
      progress: [],
      results: [],
      error: out.validation.message,
    }
    return out
  }
  if (out.input.allowed_domains.length && out.input.blocked_domains.length) {
    out.validation = {
      ok: false,
      message: 'Error: Cannot specify both allowed_domains and blocked_domains in the same request',
      errorCode: 2,
    }
    out.normalizedResult = {
      input: { query: out.input.query, allowedDomains: out.input.allowed_domains, blockedDomains: out.input.blocked_domains },
      execution: { method: 'provider_native', fallbackReason: '', cacheHit: false },
      progress: [],
      results: [],
      error: out.validation.message,
    }
    return out
  }

  const mockResults = Array.isArray(input.mock_results)
    ? input.mock_results.map((r) => ({ title: r.title, url: r.url, snippet: r.snippet || '' }))
    : [{ title: 'Provider search result', url: 'https://example.com/provider', snippet: `Result for ${out.input.query}` }]

  out.normalizedResult = {
    input: { query: out.input.query, allowedDomains: out.input.allowed_domains, blockedDomains: out.input.blocked_domains },
    execution: { method: 'provider_native', fallbackReason: '', cacheHit: false },
    progress: [
      { type: 'query_update', query: out.input.query },
      { type: 'search_results_received', count: mockResults.length },
    ],
    results: mockResults,
  }

  return out
}

function main() {
  const [, , tool, inputPath] = process.argv
  if (!tool || !inputPath) {
    console.error('usage: node run_web_contract_check.js <webfetch|websearch> <input.json>')
    process.exit(2)
  }
  const payload = JSON.parse(fs.readFileSync(path.resolve(inputPath), 'utf8'))
  if (tool === 'webfetch') {
    emit(webFetchContract(payload))
    return
  }
  if (tool === 'websearch') {
    emit(webSearchContract(payload))
    return
  }
  console.error(`unknown tool: ${tool}`)
  process.exit(2)
}

main()
