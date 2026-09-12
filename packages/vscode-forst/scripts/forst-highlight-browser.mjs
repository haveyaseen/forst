/**
 * Browser bootstrap for Mintlify docs. Bundled into docs/forst-highlight.js by sync script.
 */

/* global GRAMMAR, highlightToHtml, isForstBlock */

/**
 * @param {HTMLElement} code
 * @returns {boolean}
 */
function alreadyHighlighted(code) {
  if (code.dataset.ftHighlighted === "1") return true;
  return Boolean(
    code.querySelector(
      "span.ft-tok-keyword, span.ft-tok-string, span.ft-tok-comment, span.ft-tok-function, span.ft-tok-type"
    )
  );
}

/**
 * @param {HTMLElement} container
 * @param {string} source
 */
function applyHighlightHtml(container, source) {
  container.innerHTML = highlightToHtml(source, GRAMMAR);
}

/**
 * @param {HTMLElement} code
 */
function highlightBlock(code) {
  if (!isForstBlock(code, document) || alreadyHighlighted(code)) return;

  const lineEls = code.querySelectorAll(":scope > .line");
  if (lineEls.length > 0) {
    lineEls.forEach((line) => {
      if (!(line instanceof HTMLElement)) return;
      applyHighlightHtml(line, line.textContent ?? "");
    });
  } else {
    applyHighlightHtml(code, code.textContent ?? "");
  }

  code.dataset.ftHighlighted = "1";
}

/**
 * @param {ParentNode} root
 */
function highlightAll(root) {
  root.querySelectorAll("pre code").forEach((code) => {
    if (code instanceof HTMLElement) highlightBlock(code);
  });
}

function init() {
  highlightAll(document);
  const observer = new MutationObserver((mutations) => {
    for (const mutation of mutations) {
      mutation.addedNodes.forEach((node) => {
        if (node instanceof HTMLElement) highlightAll(node);
      });
    }
  });
  observer.observe(document.body, { childList: true, subtree: true });
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
