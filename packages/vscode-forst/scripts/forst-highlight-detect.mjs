/** @param {string | null | undefined} text */
export function isForstLabel(text) {
  const label = (text ?? "").trim();
  if (!label) return false;
  if (/^(ft|forst)$/i.test(label)) return true;
  if (/\.ft$/i.test(label)) return true;
  return false;
}

/** @param {string} source */
export function looksLikeForstSource(source) {
  if (!source.trim()) return false;
  if (/\bfunc\s+\w+\s*\([^)]*\)\s*:/.test(source)) return true;
  if (/\bfunc\s+\w+\s*\(\s*\w+\s+\{/.test(source)) return true;
  if (/\bensure\b/.test(source)) return true;
  if (/\b\w+\s+is\s+(Nil|Ok|True|False|Present)\s*\(/.test(source)) return true;
  if (/\btype\s+\w+\s*\{[\s\S]*?\w+\s*:/.test(source)) return true;
  if (/\b\w+:\s*(String|Int|Float|Bool)(\s|,|\})/.test(source)) return true;
  if (/\b(String|Int|Float|Bool|Result|Error|Array|Map|Shape)\.(Min|Max|Ok|Err|True|False|Nil)\s*\(/.test(source)) return true;
  if (/\berror\s+[A-Z]\w*/.test(source)) return true;
  return false;
}

/**
 * @param {HTMLElement} el
 * @param {number} maxDepth
 * @returns {HTMLElement[]}
 */
export function walkSelfAndAncestors(el, maxDepth) {
  /** @type {HTMLElement[]} */
  const nodes = [];
  let current = el;
  for (let depth = 0; current && depth < maxDepth; depth++, current = current.parentElement) {
    nodes.push(current);
  }
  return nodes;
}

/**
 * @param {HTMLElement} code
 * @returns {boolean}
 */
export function hasExplicitFtLanguage(code) {
  for (const node of walkSelfAndAncestors(code, 10)) {
    const lang =
      node.getAttribute("language") ||
      node.getAttribute("data-language") ||
      node.getAttribute("data-lang") ||
      "";
    if (lang === "ft" || lang === "forst") return true;
    if (/\blanguage-(?:ft|forst)\b/.test(node.className || "")) return true;
  }
  return false;
}

/**
 * @param {HTMLElement} code
 * @param {Document} doc
 * @returns {boolean}
 */
export function hasForstTabContext(code, doc) {
  for (const node of walkSelfAndAncestors(code, 12)) {
    if (node.getAttribute("role") !== "tabpanel") continue;

    const labelledBy = node.getAttribute("aria-labelledby");
    if (labelledBy) {
      const tab = doc.getElementById(labelledBy);
      if (tab && isForstLabel(tab.textContent)) return true;
    }

    const panelId = node.id;
    if (panelId) {
      const tab = doc.querySelector(`[role="tab"][aria-controls="${panelId}"]`);
      if (tab && isForstLabel(tab.textContent)) return true;
    }
  }

  for (const container of walkSelfAndAncestors(code, 15)) {
    if (typeof container.querySelectorAll !== "function") continue;
    const tabs = container.querySelectorAll('[role="tab"]');
    const panels = container.querySelectorAll('[role="tabpanel"]');
    if (tabs.length === 0 || panels.length === 0) continue;

    for (let i = 0; i < panels.length; i++) {
      const panel = panels[i];
      if (typeof panel.contains === "function" && !panel.contains(code)) continue;
      const tab = tabs[i];
      if (tab && isForstLabel(tab.textContent)) return true;
    }
  }

  return false;
}

/**
 * @param {HTMLElement} code
 * @returns {boolean}
 */
export function isPlainTextFallback(code) {
  for (const node of walkSelfAndAncestors(code, 4)) {
    const lang = node.getAttribute("language") || node.getAttribute("data-language") || "";
    if (lang === "text" || lang === "plaintext") return true;
  }
  return false;
}

/**
 * Mintlify may tag Forst-looking blocks as Go when they use package/import/func.
 * @param {HTMLElement} code
 * @returns {boolean}
 */
export function isMislabeledForstLanguage(code) {
  for (const node of walkSelfAndAncestors(code, 4)) {
    const lang = node.getAttribute("language") || node.getAttribute("data-language") || "";
    if (lang === "go") return true;
    if (/\blanguage-go\b/.test(node.className || "")) return true;
  }
  return false;
}

/**
 * @param {HTMLElement} code
 * @returns {boolean}
 */
export function hasForstCodeBlockLabel(code) {
  const closest = code.closest?.bind(code);
  if (!closest) return false;

  const root =
    closest('[data-component-part="code-block-root"]') ||
    closest('[data-component-part="code-group-root"]') ||
    closest("pre")?.parentElement;

  if (!root) return false;

  const labelSelectors = [
    '[data-component-part="code-block-language"]',
    '[data-component-part="code-block-title"]',
    '[data-component-part="code-group-tab"]',
    "button[role='tab'][aria-selected='true']",
  ];

  for (const selector of labelSelectors) {
    const el = root.querySelector(selector) || root.parentElement?.querySelector(selector);
    if (el && isForstLabel(el.textContent)) return true;
  }

  const header = root.previousElementSibling;
  if (header instanceof HTMLElement && isForstLabel(header.textContent)) return true;

  return false;
}

/**
 * @param {HTMLElement} code
 * @param {Document} doc
 * @returns {boolean}
 */
export function isForstBlock(code, doc) {
  if (hasExplicitFtLanguage(code)) return true;
  if (hasForstTabContext(code, doc)) return true;
  if (hasForstCodeBlockLabel(code)) return true;
  const source = code.textContent ?? "";
  if (!looksLikeForstSource(source)) return false;
  if (isPlainTextFallback(code)) return true;
  if (isMislabeledForstLanguage(code)) return true;
  return false;
}
