# Helmhammer

A work-in-progress compiler from a Helm Chart into Jsonnet.

## Usage

Install go-jsonnet >= v0.21.0. Then:

```
go run main.go /path/to/helm/chart/directory > your-chart.jsonnet

echo "(import 'your-chart.jsonnet')(values={ /* whatever you want */ })" > main.jsonnet

jsonnet main.jsonnet
```

## Limitations

- no support for `break` and `continue`.
- no support for channels.
- no support for the following functions in Helm:
  - `lookup`
  - `now`
- limited and/or incompatible support for the following functions in Helm:
  - `tpl`
  - `regexReplaceAll`
  - `mergeOverwrite`
- output of `toYaml` function in Helm may be different from authentic one.
- `Capabilities.APIVersions` in Helm is an object that has only `Has` field.
