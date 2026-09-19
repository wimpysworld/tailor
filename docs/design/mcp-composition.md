# MCP client composition

Status: implemented internal extension design. The [current contract](../SPECIFICATION.md#model-context-protocol-support) remains authoritative for user behaviour.

## Current boundary

`fixedManagedMCPRegistry` contains one production server, `playwright`. Each `managedMCPServerDefinition` has one `managedMCPProviderDefinition` for Claude, Codex, OpenCode, and Pi.

Tailor selects declared, enabled servers and renders one destination for each client. It sorts definitions by lexical server name before serialisation. An excluded server contributes no entry. Tailor still renders a client destination when another server is enabled. If no servers are enabled, Tailor does not select or inspect a destination. Package fragments use the separate existing Nix lifecycle, so adding a server definition does not select a package.

Registry validation requires unique server names, unique provider definitions, all four providers, and valid UTF-8 strings without Unicode control characters. It rejects unsupported provider fields and unresolved `[[TAILOR_` tokens. Renderers preserve literal strings, including the Pi `HTTPS_PROXY` shell expression.

The four embedded starters remain byte-parity references for the production registry and sources for manual copying. Runtime composition does not read them. The OpenCode V1 starter format remains unchanged.

Tailor creates a composed client file only when the destination is missing. It does not parse, adopt, merge, or replace an existing regular file or final symlink. When all current server declarations are absent or false, Tailor does not inspect a destination. A false declaration can remove only its owned package fragment and can produce the existing package warning.

## Add a server

1. Add its configuration state without changing unrelated external schemas.
2. Add one server entry to `fixedManagedMCPRegistry`.
3. Define exact output fields for all four providers.
4. Keep package selection in the managed Nix fragment registry.
5. Update each embedded starter to match the production rendering when the production selection changes.
6. Add composition tests for lexical order, false and absent states, validation, escaping, and byte parity.
7. Update the specification and user configuration reference.

Do not add a server until all four client definitions and its package lifecycle are known.

## Future work

Provider additions need a destination mapping, field validator, renderer, starter reference, and tests. A format-version change, including a successor to OpenCode V1, needs an explicit version policy and migration design.

Shared-file adoption remains future work. See the [MCP entry adoption contract](mcp-adoption.md).
