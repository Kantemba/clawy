import { useEffect } from "react"

const THEME_STYLE_ID = "hljs-theme-style"
const THEME_STYLE_OWNER_ATTR = "data-clawy-highlight-theme"
const THEME_STYLE_OWNER_VALUE = "true"
const MANAGED_THEME_STYLE_SELECTOR = `style[${THEME_STYLE_OWNER_ATTR}="${THEME_STYLE_OWNER_VALUE}"]`
const ID_THEME_STYLE_SELECTOR = `style#${THEME_STYLE_ID}`

/**
 * Monochrome highlight.js palette.
 *
 * The UI is strictly black and white, so syntax highlighting relies on
 * weight, style and tonal opacity instead of hue.
 */
const MONOCHROME_THEME_CSS = `
.hljs {
  color: inherit;
  background: transparent;
}

.hljs-comment,
.hljs-quote {
  font-style: italic;
  opacity: 0.55;
}

.hljs-meta {
  opacity: 0.6;
}

.hljs-keyword,
.hljs-selector-tag,
.hljs-literal,
.hljs-type,
.hljs-selector-id,
.hljs-selector-class,
.hljs-built_in,
.hljs-doctag,
.hljs-name,
.hljs-strong {
  font-weight: 700;
}

.hljs-title,
.hljs-section,
.hljs-function .hljs-title,
.hljs-title.function_,
.hljs-title.class_ {
  font-weight: 600;
}

.hljs-attr,
.hljs-attribute,
.hljs-variable,
.hljs-template-variable,
.hljs-property,
.hljs-number,
.hljs-symbol,
.hljs-bullet,
.hljs-regexp,
.hljs-link {
  opacity: 0.7;
}

.hljs-string,
.hljs-template-tag {
  opacity: 0.85;
}

.hljs-deletion {
  text-decoration: line-through;
  opacity: 0.55;
}

.hljs-addition {
  text-decoration: underline;
}

.hljs-emphasis {
  font-style: italic;
}

.hljs-strong {
  font-weight: 700;
}
`

const CHAT_CODE_BLOCK_OVERRIDES = `
[data-clawy-code-block] .hljs {
  background: transparent !important;
}

[data-clawy-code-block] pre code.hljs,
[data-clawy-code-block] code.hljs {
  padding: 0 !important;
  background: transparent !important;
}
`

function getOrCreateThemeStyleElement(): HTMLStyleElement {
  const managedStyleElement = document.head.querySelector<HTMLStyleElement>(
    MANAGED_THEME_STYLE_SELECTOR,
  )
  if (managedStyleElement) {
    return managedStyleElement
  }

  const existingStyleElement = document.querySelector<HTMLStyleElement>(
    ID_THEME_STYLE_SELECTOR,
  )
  if (existingStyleElement) {
    existingStyleElement.setAttribute(
      THEME_STYLE_OWNER_ATTR,
      THEME_STYLE_OWNER_VALUE,
    )
    return existingStyleElement
  }

  const conflictingElement = document.getElementById(THEME_STYLE_ID)
  const styleElement = document.createElement("style")
  if (!conflictingElement) {
    styleElement.id = THEME_STYLE_ID
  }

  // Leave conflicting non-style nodes untouched and track the injected style explicitly.
  styleElement.setAttribute(THEME_STYLE_OWNER_ATTR, THEME_STYLE_OWNER_VALUE)
  document.head.appendChild(styleElement)

  return styleElement
}

export function useHighlightTheme() {
  useEffect(() => {
    const styleElement = getOrCreateThemeStyleElement()
    styleElement.textContent = `${MONOCHROME_THEME_CSS}\n${CHAT_CODE_BLOCK_OVERRIDES}`
  }, [])
}
