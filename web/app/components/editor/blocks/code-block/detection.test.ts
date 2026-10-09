import { describe, it } from "vitest"
import { detectLanguage } from "./detection"

const SNIPPETS = [
	{
		name: 'detects bash in a snippet starting "#!/usr/bin/env bash"',
		input:
			'#!/usr/bin/env bash\nset -euo pipefail\n\nfor f in *.log; do\n  echo "processing $f"\n  gzip "$f"\ndone',
		expected: "bash",
	},
	{
		name: 'detects bash in a snippet starting "docker compose up -d"',
		input: "docker compose up -d\nmake e2e-dev\npnpm install",
		expected: "bash",
	},
	{
		name: 'detects c in a snippet starting "#include <stdio.h>"',
		input:
			'#include <stdio.h>\n\nint main(void) {\n    printf("hello\\n");\n    return 0;\n}',
		expected: "c",
	},
	{
		name: 'detects cpp in a snippet starting "#include <iostream>"',
		input:
			"#include <iostream>\n#include <vector>\n\nint main() {\n    std::vector<int> v = {1, 2, 3};\n    for (auto x : v) {\n        std::cout << x << std::endl;\n    }\n    return 0;\n}",
		expected: "cpp",
	},
	{
		name: 'detects csharp in a snippet starting "using System;"',
		input:
			"using System;\nusing System.Collections.Generic;\n\nnamespace App\n{\n    public class Program\n    {\n        public static void Main(string[] args)\n        {\n            var items = new List<string>();\n            foreach (var item in items)\n            {\n                Console.WriteLine(item);\n            }\n        }\n    }\n}",
		expected: "csharp",
	},
	{
		name: 'detects csharp in a snippet starting "public class User"',
		input:
			'public class User\n{\n    public string Id { get; set; }\n    public string Name { get; set; }\n\n    public async Task<User> LoadAsync(HttpClient client)\n    {\n        var json = await client.GetStringAsync("/me");\n        return JsonSerializer.Deserialize<User>(json);\n    }\n}',
		expected: "csharp",
	},
	{
		name: 'detects css in a snippet starting ".button {"',
		input:
			".button {\n  color: red;\n  padding: 0.5rem 1rem;\n}\n\n.button:hover {\n  color: blue;\n}",
		expected: "css",
	},
	{
		name: 'detects curl in a snippet starting "curl -X POST https://api.example"',
		input:
			'curl -X POST https://api.example.com/v1/items \\\n  -H "Authorization: Bearer $TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d \'{"name": "x"}\'',
		expected: "curl",
	},
	{
		name: 'detects diff in a snippet starting "--- a/file.txt"',
		input:
			"--- a/file.txt\n+++ b/file.txt\n@@ -1,3 +1,3 @@\n-old line\n+new line\n context",
		expected: "diff",
	},
	{
		name: 'detects go in a snippet starting "package main"',
		input:
			'package main\n\nimport (\n\t"fmt"\n\t"net/http"\n)\n\ntype Server struct {\n\taddr string\n}\n\nfunc (s *Server) Run() error {\n\tmux := http.NewServeMux()\n\tmux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {\n\t\tfmt.Fprintln(w, "hi")\n\t})\n\n\tif err := http.ListenAndServe(s.addr, mux); err != nil {\n\t\treturn err\n\t}\n\n\treturn nil\n}',
		expected: "go",
	},
	{
		name: 'detects go in a snippet starting "func Sum(values []int) int {"',
		input:
			"func Sum(values []int) int {\n\ttotal := 0\n\tfor _, v := range values {\n\t\ttotal += v\n\t}\n\n\treturn total\n}",
		expected: "go",
	},
	{
		name: 'detects go in a snippet starting "type User struct {"',
		input:
			'type User struct {\n\tID        string    `json:"id"`\n\tName      string    `json:"name"`\n\tCreatedAt time.Time `json:"created_at"`\n}',
		expected: "go",
	},
	{
		name: 'detects graphql in a snippet starting "query GetUser($id: ID!) {"',
		input:
			"query GetUser($id: ID!) {\n  user(id: $id) {\n    id\n    name\n    posts(first: 10) {\n      title\n    }\n  }\n}",
		expected: "graphql",
	},
	{
		name: 'detects ini in a snippet starting "[server]"',
		input:
			"[server]\nhost = 0.0.0.0\nport = 8080\n\n[database]\nurl = postgres://localhost/db",
		expected: "ini",
	},
	{
		name: 'detects java in a snippet starting "import java.util.List;"',
		input:
			'import java.util.List;\n\npublic class Main {\n    public static void main(String[] args) {\n        List<String> items = List.of("a", "b");\n        for (String item : items) {\n            System.out.println(item);\n        }\n    }\n}',
		expected: "java",
	},
	{
		name: 'detects javascript in a snippet starting "const express = require(\\"express"',
		input:
			'const express = require("express")\nconst app = express()\n\napp.get("/", (req, res) => {\n\tres.send("hello")\n})\n\napp.listen(3000, () => console.log("up"))',
		expected: "javascript",
	},
	{
		name: 'detects json in a snippet starting "{"',
		input:
			'{\n  "name": "oxynote",\n  "version": "1.0.0",\n  "private": true,\n  "scripts": {\n    "dev": "nuxt dev",\n    "build": "nuxt build"\n  },\n  "tags": ["a", "b"],\n  "count": 3\n}',
		expected: "json",
	},
	{
		name: 'detects json in a snippet starting "{\\"id\\": 12, \\"ok\\": true}"',
		input: '{"id": 12, "ok": true}',
		expected: "json",
	},
	{
		name: 'detects json in a snippet starting "["',
		input: '[\n  { "id": 1, "name": "a" },\n  { "id": 2, "name": "b" }\n]',
		expected: "json",
	},
	{
		name: 'detects kotlin in a snippet starting "fun main() {"',
		input:
			'fun main() {\n    val items = listOf("a", "b")\n    for (item in items) {\n        println(item)\n    }\n}',
		expected: "kotlin",
	},
	{
		name: 'detects makefile in a snippet starting "CC = gcc"',
		input:
			"CC = gcc\nCFLAGS = -Wall -O2\n\napp: main.o util.o\n\t$(CC) $(CFLAGS) -o $@ $^\n\nclean:\n\trm -f *.o app",
		expected: "makefile",
	},
	{
		name: 'detects markdown in a snippet starting "# Title"',
		input:
			"# Title\n\nSome **bold** text and a [link](https://example.com).\n\n- one\n- two",
		expected: "markdown",
	},
	{
		name: 'detects php in a snippet starting "<?php"',
		input:
			"<?php\n\nfunction total(array $values): int {\n    $sum = 0;\n    foreach ($values as $v) {\n        $sum += $v;\n    }\n    return $sum;\n}",
		expected: "php",
	},
	{
		name: 'detects plaintext in a snippet starting "sum(rate(http_requests_total{job"',
		input: 'sum(rate(http_requests_total{job="api"}[5m])) by (status)',
		expected: "plaintext",
	},
	{
		name: 'detects plaintext in a snippet starting "This is just a note about the de"',
		input:
			"This is just a note about the deployment.\nNothing here is code at all.",
		expected: "plaintext",
	},
	{
		name: 'detects plaintext in a snippet starting "Remember to rotate the keys befo"',
		input: "Remember to rotate the keys before Friday, and tell the team.",
		expected: "plaintext",
	},
	{
		name: 'detects plaintext in a snippet starting "GET /api/users/42"',
		input: "GET /api/users/42\nHost: example.com\nAccept: application/json",
		expected: "plaintext",
	},
	{
		name: 'detects python in a snippet starting "import os"',
		input:
			'import os\nfrom typing import List\n\ndef total(values: List[int]) -> int:\n    result = 0\n    for v in values:\n        result += v\n    return result\n\nif __name__ == "__main__":\n    print(total([1, 2, 3]))',
		expected: "python",
	},
	{
		name: 'detects ruby in a snippet starting "class User"',
		input:
			'class User\n  attr_reader :name\n\n  def initialize(name)\n    @name = name\n  end\n\n  def greet\n    puts "hi #{@name}"\n  end\nend',
		expected: "ruby",
	},
	{
		name: 'detects rust in a snippet starting "use std::collections::HashMap;"',
		input:
			'use std::collections::HashMap;\n\nfn main() {\n    let mut map: HashMap<String, i32> = HashMap::new();\n    map.insert("a".to_string(), 1);\n    for (k, v) in &map {\n        println!("{k}: {v}");\n    }\n}',
		expected: "rust",
	},
	{
		name: 'detects shell in a snippet starting "$ docker compose up -d"',
		input: "$ docker compose up -d\n$ make e2e-dev",
		expected: "shell",
	},
	{
		name: 'detects sql in a snippet starting "SELECT u.id, u.name, count(o.id)"',
		input:
			"SELECT u.id, u.name, count(o.id) AS orders\nFROM users u\nLEFT JOIN orders o ON o.user_id = u.id\nWHERE u.active = true\nGROUP BY u.id\nORDER BY orders DESC;",
		expected: "sql",
	},
	{
		name: 'detects swift in a snippet starting "import Foundation"',
		input:
			'import Foundation\n\nstruct User {\n    let name: String\n}\n\nfunc greet(_ user: User) -> String {\n    return "hi \\(user.name)"\n}',
		expected: "swift",
	},
	{
		name: 'detects typescript in a snippet starting "interface User {"',
		input:
			'interface User {\n\tid: string\n\tname: string\n}\n\nexport async function loadUser(id: string): Promise<User> {\n\tconst res = await fetch(`/api/users/${id}`)\n\tif (!res.ok) {\n\t\tthrow new Error("failed")\n\t}\n\n\treturn (await res.json()) as User\n}',
		expected: "typescript",
	},
	{
		name: 'detects xml in a snippet starting "<!DOCTYPE html>"',
		input:
			'<!DOCTYPE html>\n<html>\n  <head><title>x</title></head>\n  <body>\n    <div class="a">hi</div>\n  </body>\n</html>',
		expected: "xml",
	},
	{
		name: 'detects yaml in a snippet starting "services:"',
		input:
			'services:\n  web:\n    image: nginx:latest\n    ports:\n      - "8080:80"\n    environment:\n      - KEY=value',
		expected: "yaml",
	},
]

// the highlighter's keyword count alone names another language for this one
const GO_WITH_SHARED_KEYWORDS =
	'var (\n\tErrNotFound = errors.New("not found")\n\tErrInvalid  = errors.New("invalid")\n)\n\ntype Store interface {\n\tGet(ctx context.Context, id string) (User, error)\n\tPut(ctx context.Context, u User) error\n}'

const NO_CLEAR_LEAD =
	'public := os.Getenv("PUBLIC_URL")\nvar client = &http.Client{Timeout: 5 * time.Second}\nvar result string\nif public == "" {\n\tresult = "none"\n}\nnew := strings.TrimSpace(result)'

const WEAK_EVIDENCE =
	'sum(rate(http_requests_total{job="api"}[5m])) by (status)'

describe("detectLanguage", () => {
	it.for(SNIPPETS)("$name", ({ input, expected }, { expect }) => {
		expect(detectLanguage(input)).toBe(expected)
	})

	it("detects go where the shared keywords favour another language", ({
		expect,
	}) => {
		expect(detectLanguage(GO_WITH_SHARED_KEYWORDS)).toBe("go")
	})

	it("detects json written on one line", ({ expect }) => {
		expect(detectLanguage('{"id": 12, "ok": true}')).toBe("json")
	})

	it("falls back to plaintext for blank code", ({ expect }) => {
		expect(detectLanguage("  \n ")).toBe("plaintext")
	})

	it("falls back to plaintext when no language has a clear lead", ({
		expect,
	}) => {
		expect(detectLanguage(NO_CLEAR_LEAD)).toBe("plaintext")
	})

	it("falls back to plaintext when the evidence is weak", ({ expect }) => {
		expect(detectLanguage(WEAK_EVIDENCE)).toBe("plaintext")
	})
})
