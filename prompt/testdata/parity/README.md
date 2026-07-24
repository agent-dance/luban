# Prompt parity golden fixtures

These fixtures lock down Go prompt parity behavior without invoking the TypeScript runtime or any network services.

To intentionally update the golden snapshots after a reviewed prompt change:

```sh
UPDATE_PROMPT_GOLDENS=1 go test ./prompt
go test ./prompt
```

Review the resulting `prompt/testdata/parity/golden/*.json` diff before committing. Do not update goldens just to make an unexpected failure pass; first confirm the prompt ordering, block metadata, cache scope, and context injection behavior are intentionally changing.
