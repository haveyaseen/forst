import assert from "node:assert";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import {
  hasExplicitFtLanguage,
  hasForstTabContext,
  isForstBlock,
  isForstLabel,
  isPlainTextFallback,
  looksLikeForstSource,
} from "../scripts/forst-highlight-detect.mjs";
import {
  highlightToHtml,
  renderHtml,
  scopeToClass,
  tokenize,
} from "../scripts/forst-tokenize.mjs";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.join(__dirname, "..", "..", "..");
const grammarPath = path.join(repoRoot, "docs", "languages", "forst.json");
const highlightJsPath = path.join(repoRoot, "docs", "forst-highlight.js");
const helloSnippetPath = path.join(repoRoot, "docs", "snippets", "hello.ft.mdx");

const grammar = JSON.parse(fs.readFileSync(grammarPath, "utf8"));

/** @param {Record<string, string | null>} attrs */
function el(tag, attrs = {}, children = []) {
  /** @type {HTMLElement & { _children: HTMLElement[], _parent: HTMLElement | null }} */
  const node = {
    tagName: tag.toUpperCase(),
    className: attrs.class || "",
    id: attrs.id || "",
    _attrs: attrs,
    _children: children,
    _parent: null,
    textContent: "",
    dataset: {},
    parentElement: null,
    getAttribute(name) {
      return this._attrs[name] ?? null;
    },
    contains(target) {
      if (target === this) return true;
      for (const child of this._children ?? []) {
        if (child.contains?.(target)) return true;
      }
      return false;
    },
    querySelector(selector) {
      return null;
    },
    querySelectorAll(selector) {
      /** @type {HTMLElement[]} */
      const out = [];
      const visit = (node) => {
        if (selector === '[role="tab"]' && node.getAttribute("role") === "tab") out.push(node);
        if (selector === '[role="tabpanel"]' && node.getAttribute("role") === "tabpanel") out.push(node);
        for (const child of node._children ?? []) visit(child);
      };
      visit(this);
      return out;
    },
  };
  for (const child of children) {
    child._parent = node;
    child.parentElement = node;
  }
  node.textContent = children.map((c) => c.textContent ?? "").join("");
  return node;
}

/** @param {HTMLElement} node */
function setDoc(node, doc) {
  node.ownerDocument = doc;
  for (const child of node._children ?? []) {
    if (child instanceof Object && "getAttribute" in child) setDoc(child, doc);
  }
}

function makeDoc(root) {
  const doc = {
    body: root,
    getElementById(id) {
      return findById(root, id);
    },
    querySelector(selector) {
      if (selector.startsWith("[role=\"tab\"][aria-controls=\"")) {
        const panelId = selector.match(/aria-controls="([^"]+)"/)?.[1];
        return findTabByPanel(root, panelId);
      }
      return null;
    },
  };
  setDoc(root, doc);
  return doc;
}

/** @param {HTMLElement} node @param {string} id */
function findById(node, id) {
  if (node.id === id) return node;
  for (const child of node._children ?? []) {
    const hit = findById(child, id);
    if (hit) return hit;
  }
  return null;
}

/** @param {HTMLElement} node @param {string | undefined} panelId */
function findTabByPanel(node, panelId) {
  if (node.getAttribute("role") === "tab" && node.getAttribute("aria-controls") === panelId) return node;
  for (const child of node._children ?? []) {
    const hit = findTabByPanel(child, panelId);
    if (hit) return hit;
  }
  return null;
}

function classesIn(html) {
  return [...html.matchAll(/class="([^"]+)"/g)].map((m) => m[1]);
}

function htmlContainsClass(html, cls) {
  return html.includes(`class="${cls}"`);
}

test("scopeToClass maps common Forst scopes", () => {
  assert.equal(scopeToClass("keyword.declaration"), "ft-tok-keyword");
  assert.equal(scopeToClass("comment.line.double-slash.forst"), "ft-tok-comment");
  assert.equal(scopeToClass("string.quoted.double.forst"), "ft-tok-string");
  assert.equal(scopeToClass("entity.name.function"), "ft-tok-function");
  assert.equal(scopeToClass("support.type.primitive"), "ft-tok-type");
});

test("tokenize keeps camelCase call names intact (placeOrder, not Order)", () => {
  const source = [
    "x := placeOrder({",
    '\tstockKeepingUnit: "ITEM-1",',
    "\tquantity:         2,",
    "})",
  ].join("\n");
  const tokens = tokenize(source, grammar);
  const slices = tokens.map((t) => ({
    text: source.slice(t.start, t.end),
    cls: scopeToClass(t.scope),
  }));
  const placeOrder = slices.find((t) => t.text === "placeOrder");
  assert.ok(placeOrder, `expected a single placeOrder token, got ${JSON.stringify(slices)}`);
  assert.equal(placeOrder.cls, "ft-tok-function");
  assert.equal(
    slices.some((t) => t.text === "Order" || t.text.startsWith("Order(")),
    false,
    `must not split placeOrder into Order; got ${JSON.stringify(slices)}`
  );
  const html = highlightToHtml(source, grammar);
  assert.match(html, /<span class="ft-tok-function">placeOrder<\/span>/);
  const str = slices.find((t) => t.text === '"ITEM-1"');
  assert.ok(str, `expected a full string token, got ${JSON.stringify(slices)}`);
  assert.equal(str.cls, "ft-tok-string");
  assert.ok(htmlContainsClass(html, "ft-tok-string"), "string literal should be string");
});

test("tokenize highlights func keyword and String type in hello snippet body", () => {
  const source = [
    "package main",
    "",
    "func greet(): String {",
    '\treturn "Hello, World!"',
    "}",
  ].join("\n");
  const html = highlightToHtml(source, grammar);
  assert.ok(htmlContainsClass(html, "ft-tok-keyword"), "func should be keyword");
  assert.ok(htmlContainsClass(html, "ft-tok-type"), "String should be type");
  assert.ok(htmlContainsClass(html, "ft-tok-string"), "string literal should be string");
  assert.ok(html.includes("func"));
  assert.ok(html.includes("greet"));
});

test("tokenize highlights line comments", () => {
  const source = "// setup\nfunc main() {}";
  const html = highlightToHtml(source, grammar);
  assert.ok(htmlContainsClass(html, "ft-tok-comment"), "comment should be highlighted");
  assert.ok(html.includes("// setup"));
});

test("tokenize highlights ensure keyword", () => {
  const source = "ensure x is Ok()";
  const html = highlightToHtml(source, grammar);
  assert.ok(htmlContainsClass(html, "ft-tok-keyword"));
  assert.match(html, /ensure/);
});

test("renderHtml escapes HTML in source", () => {
  const html = renderHtml("<script>", []);
  assert.equal(html, "&lt;script&gt;");
});

test("hello.ft.mdx Forst tab content gets keyword and string spans", () => {
  const snippet = fs.readFileSync(helloSnippetPath, "utf8");
  const match = snippet.match(/```ft\n([\s\S]*?)```/);
  assert.ok(match, "hello snippet should contain ```ft block");
  const source = match[1];
  const html = highlightToHtml(source, grammar);
  const classes = classesIn(html);
  assert.ok(classes.includes("ft-tok-keyword"), `expected keyword class, got ${classes.join(", ")}`);
  assert.ok(classes.includes("ft-tok-string"), `expected string class, got ${classes.join(", ")}`);
});

const verifyPasswordSource = [
  "package auth",
  "",
  'import "golang.org/x/crypto/bcrypt"',
  "",
  "func VerifyPassword(input {",
  "  plainPassword: String,",
  "  passwordHash: String",
  "}) {",
  "  compareErr := bcrypt.CompareHashAndPassword(hashBytes, plainBytes)",
  "  if compareErr is Nil() {",
  "    return { valid: true }",
  "  }",
  "  return { valid: false }",
  "}",
].join("\n");

test("looksLikeForstSource detects Forst return types and ensure", () => {
  assert.equal(looksLikeForstSource('func greet(): String {\n  return "hi"\n}'), true);
  assert.equal(looksLikeForstSource("ensure x is Ok()"), true);
  assert.equal(looksLikeForstSource(verifyPasswordSource), true);
  assert.equal(looksLikeForstSource('package main\nimport "fmt"\nfunc main() {}'), false);
});

test("isForstBlock detects mislabeled go blocks with Forst syntax", () => {
  const code = el("code", { language: "go", class: "language-go" });
  code.textContent = verifyPasswordSource;
  const root = el("pre", { language: "go", class: "language-go" }, [code]);
  const doc = makeDoc(root);
  assert.equal(isForstBlock(code, doc), true);
});

test("isForstLabel accepts Forst tab titles and .ft filenames", () => {
  assert.equal(isForstLabel("Forst"), true);
  assert.equal(isForstLabel("hello.ft"), true);
  assert.equal(isForstLabel("Generated Go"), false);
});

test("hasForstTabContext detects code inside Forst tab panel", () => {
  const tabForst = el("button", { role: "tab", id: "tab-forst", "aria-controls": "panel-forst" });
  tabForst.textContent = "Forst";
  const panel = el("div", { role: "tabpanel", id: "panel-forst", "aria-labelledby": "tab-forst" }, [
    el("pre", { language: "text" }, [
      el("code", { language: "text" }, [el("span", { class: "line" })]),
    ]),
  ]);
  const root = el("div", {}, [tabForst, panel]);
  const code = panel._children[0]._children[0];
  const doc = makeDoc(root);
  assert.equal(hasForstTabContext(code, doc), true);
  assert.equal(isForstBlock(code, doc), true);
});

test("isPlainTextFallback detects Mintlify unknown-language blocks", () => {
  const code = el("code", { language: "text" });
  code.textContent = "func greet(): String {}";
  const root = el("pre", { language: "text" }, [code]);
  const doc = makeDoc(root);
  assert.equal(isPlainTextFallback(code), true);
  assert.equal(hasExplicitFtLanguage(code), false);
  assert.equal(isForstBlock(code, doc), true);
});

test("docs/forst-highlight.js targets Mintlify text fallback and Forst tabs", () => {
  const js = fs.readFileSync(highlightJsPath, "utf8");
  assert.match(js, /function looksLikeForstSource\(/);
  assert.match(js, /function hasForstTabContext\(/);
  assert.match(js, /:scope > \.line/);
  assert.doesNotMatch(js, /span\.line\)\) return true/);
});

test("docs/forst-highlight.css sets --shiki-dark for Mintlify dark mode", () => {
  const cssPath = path.join(repoRoot, "docs", "forst-highlight.css");
  const css = fs.readFileSync(cssPath, "utf8");
  assert.match(css, /html\.dark pre code \.ft-tok-function/);
  assert.match(css, /--shiki-dark:/);
  assert.doesNotMatch(css, /:root\.dark pre code/);
});

test("docs/forst-highlight.js is generated and embeds grammar name ft", () => {
  const js = fs.readFileSync(highlightJsPath, "utf8");
  assert.match(js, /function tokenize\(/);
  assert.match(js, /function highlightToHtml\(/);
  assert.match(js, /const GRAMMAR = \{/);
  assert.match(js, /"name":"ft"/);
  assert.match(js, /dataset\.ftHighlighted/);
});
