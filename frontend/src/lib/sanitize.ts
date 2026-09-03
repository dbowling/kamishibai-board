import DOMPurify from 'dompurify';

/**
 * Card instructions are rich text, and any team member can write them.
 *
 * That makes them untrusted input from every other member's point of view: a
 * shared board is exactly the place where one person's content is rendered in
 * everybody else's browser. Instructions are therefore sanitised before being
 * inserted as HTML, and links are checked separately with safeUrl.
 */

/**
 * Tags worth keeping for a set of instructions. Deliberately no media, no forms,
 * no iframes: this is prose with links and lists, and a narrow allowlist is
 * easier to reason about than a broad denylist.
 */
const ALLOWED_TAGS = [
  // '#text' is not optional. When ALLOWED_TAGS is supplied explicitly, DOMPurify
  // treats text nodes as just another node type to filter, so omitting it strips
  // every piece of text and leaves behind a skeleton of empty tags.
  '#text',
  // Editors habitually wrap content in divs and spans. They carry no meaning
  // here, but they have to be allowed or the text inside them goes with them.
  'div',
  'p',
  'br',
  'hr',
  'strong',
  'b',
  'em',
  'i',
  'u',
  's',
  'code',
  'pre',
  'blockquote',
  'ul',
  'ol',
  'li',
  'a',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'table',
  'thead',
  'tbody',
  'tr',
  'th',
  'td',
  'span',
];

const ALLOWED_ATTR = ['href', 'title', 'target', 'rel'];

/** Sanitises card instructions for rendering as HTML. */
export function sanitizeInstructions(html: string): string {
  if (!html) return '';

  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS,
    ALLOWED_ATTR,
    // Block every URI scheme except the few that make sense in a runbook.
    ALLOWED_URI_REGEXP: /^(?:https?:|mailto:|#|\/)/i,
    // Unwrap unknown tags but keep the words inside them, so a stray <marquee>
    // costs the tag and not the sentence. Script and style are exempt: they are
    // in DOMPurify's default FORBID_CONTENTS, so their bodies are discarded
    // rather than surfaced as visible text.
    KEEP_CONTENT: true,
    RETURN_TRUSTED_TYPE: false,
  });
}

/**
 * Returns the URL if it is safe to link to, otherwise null.
 *
 * A card's links are structured data rather than markup, so they never pass
 * through DOMPurify. Without this check, a `javascript:` URL in a link would run
 * on click for every teammate who opened the card.
 */
export function safeUrl(url: string): string | null {
  const trimmed = url?.trim();
  if (!trimmed) return null;

  // Relative and anchor links stay on our own origin, so they are fine.
  if (trimmed.startsWith('/') || trimmed.startsWith('#')) {
    return trimmed;
  }

  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    return null;
  }

  if (parsed.protocol === 'http:' || parsed.protocol === 'https:' || parsed.protocol === 'mailto:') {
    // Return what the author wrote, not URL's normalised form. Normalising would
    // quietly turn "https://example.test" into "https://example.test/", which is
    // harmless but surprising when the value is shown back to them for editing.
    // The security decision is the protocol check above, and that has been made.
    return trimmed;
  }
  return null;
}

/** A short, readable form of a URL for display next to a link label. */
export function displayHost(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return '';
  }
}
