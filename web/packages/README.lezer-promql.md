Original repo: https://github.com/prometheus/prometheus/tree/main/web/ui/module/lezer-promql
License: https://github.com/prometheus/prometheus/blob/main/LICENSE

The purpose or the forked lezer-promql is to add dynamic value placeholders,
like `$__interval`

### The lezer-promql was forked using:
```
mkdir -p packages/lezer-promql
cd packages/lezer-promql

# Initialize git and add prometheus as remote
git init
git remote add origin https://github.com/prometheus/prometheus.git

# Enable sparse checkout and set the subfolder
git sparse-checkout init --cone
git sparse-checkout set web/ui/module/lezer-promql

# Pull only that folder (use a specific tag for reproducibility)
git fetch origin main --depth 1
git checkout origin/main -- web/ui/module/lezer-promql

# Move files up and clean up
mv web/ui/module/lezer-promql/* .
rm -rf web .git
```

### Keep the license files
`LICENSE` (Apache-2.0) and `NOTICE` are redistributed with the grammar and
must survive a re-sync. `NOTICE` also lists the changes below — update it
when they change.

### In package.json, rename the package:
`"name": "@oxynote/lezer-promql",`

### Changes

1. Grammar change in src/promql.grammar

Add durationPlaceholder token (insert after line 283, before number):

```
@tokens {
  whitespace { std.whitespace+ }
  LineComment { "#" ![\n]* }

+ // Grafana-style duration placeholders like $__interval, $__range, $__rate_interval
+ DynamicDurationPlaceholder { "$__" $[a-zA-Z_]+ }

  number {
      (std.digit+ (("_")? std.digit)* ("." std.digit+ (("_")? std.digit)*)? | "." std.digit+ (("_")? std.digit)*) (("e" | "E") ("+" | "-")? std.digit+ (("_")? std.digit)*)? |
-     "0x" (std.digit | $[a-fA-F])+ | duration
+     "0x" (std.digit | $[a-fA-F])+ | duration | DynamicDurationPlaceholder
  }
```

2. Highlight change in src/highlight.js

Add styling for the placeholder token (insert after line 21):

```
export const promQLHighLight = styleTags({
    LineComment: tags.comment,
    LabelName: tags.labelName,
    StringLiteral: tags.string,
    NumberDurationLiteral: tags.number,
    NumberDurationLiteralInDurationContext: tags.number,
+   DynamicDurationPlaceholder: tags.number,
    Identifier: tags.variableName,
```

That's it! After these two changes, run:

```
cd packages/lezer-promql
npm install
npm run build
```