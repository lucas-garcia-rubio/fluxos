# JSON output schema v1

`fluxos trace --format=json` writes one indented JSON object followed by a newline. The
top-level `schemaVersion` is the integer `1`. There are no omitted JSON properties: empty
values are represented according to the rules below.

For the same source tree, target, and options, the values and array order are deterministic.
This includes node IDs, nodes, edges, dispatch metadata, and truncations. The format is a
snapshot of conservative analysis; it does not promise that every Java call can be resolved.

## Top-level object

| Property | Type | Meaning |
| --- | --- | --- |
| `schemaVersion` | integer | Always `1` for this schema. |
| `target` | object | Root execution. |
| `nodes` | array of objects | Ordered graph nodes. |
| `edges` | array of objects | Ordered directed calls. |
| `truncations` | array of objects | Ordered records of work stopped by a limit. |

All three arrays are always present and are `[]` when empty, never `null`.

## Shared objects

An **execution** object has:

| Property | Type |
| --- | --- |
| `method` | object |
| `runtimeType` | string |

A **method** object has `type`, `method`, and `signature`, all strings.

A **call** object has:

| Property | Type | Values |
| --- | --- | --- |
| `kind` | string | `invocation`, `objectCreation`, `thisConstructor`, `superConstructor`, `methodReference`, or `constructorReference` |
| `file` | string | Logical source file. |
| `line` | integer | Source line; zero is possible when unavailable. |
| `startByte` | non-negative integer | Source byte offset; zero is possible when unavailable. |
| `receiver` | string | Receiver text; empty when unavailable. |
| `methodName` | string | Method/constructor name; empty when unavailable. |

## Nodes

Each node has:

| Property | Type |
| --- | --- |
| `id` | string |
| `execution` | execution object |
| `kind` | string |
| `label` | string |
| `note` | string |
| `candidates` | array of strings |

`kind` is one of `method`, `external`, `unresolved`, `noImplementation`, `ambiguousType`,
`ambiguousOverload`, or `ambiguousImplementation`. `candidates` contains implementation
FQCNs and is always an array.

## Edges and dispatch sites

Each edge has `from` and `to` (strings containing node IDs), `call` (call object),
`dispatchSite` (dispatch-site object or `null`), and `cycle` (boolean).

A dispatch-site object has `id` (string), `caller` (execution object), `receiverFQCN` (string),
`method` (string), `signature` (string), `call` (call object), and `candidates` (array of
candidate objects). Each candidate has `implementationFQCN` (string), `target` (method object),
`kind` (string), and `note` (string). Candidate `kind` is one of `concrete`, `external`,
`unresolved`, `noImplementation`, `ambiguousType`, `ambiguousOverload`, or
`ambiguousImplementation`.

## Truncations

Each truncation has `id` (string), `kind` (string), `caller` (execution object), `call` (call
object), `omitted` (integer), and `note` (string). `kind` is one of `maxDepth`, `maxNodes`,
`maxImpls`, or `discoveryLimit`.

The JSON renderer always emits strings, including empty strings when a string value is absent;
numeric fields use zero when unavailable; arrays use `[]`; and an absent dispatch site uses
`null`. These are the only absence representations in schema v1.

`--include-unresolved=false` removes unresolved nodes and incident edges from the projection,
but preserves **all** truncation records, including records unrelated to those nodes.
Schema v1 does not expose renderer-specific presentation; Mermaid and DOT labels may evolve
without changing this JSON contract.
