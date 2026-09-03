import { describe, expect, it } from 'vitest';
import { parseChecklist, parseLinks, toParagraphs } from './NewCardDialog';

describe('parseLinks', () => {
  it('parses label and url pairs', () => {
    const result = parseLinks('Grafana | https://grafana.example.test/d/backups');
    expect(result.accepted).toEqual([
      { label: 'Grafana', url: 'https://grafana.example.test/d/backups' },
    ]);
    expect(result.rejected).toEqual([]);
  });

  it('falls back to the url as the label', () => {
    const result = parseLinks('https://example.test/runbook');
    expect(result.accepted[0]?.label).toBe('https://example.test/runbook');
  });

  it('ignores blank lines', () => {
    const result = parseLinks('\n\nA | https://a.example.test\n\n');
    expect(result.accepted).toHaveLength(1);
  });

  it('handles a label containing a pipe by splitting on the last one', () => {
    const result = parseLinks('A | B | https://c.example.test');
    expect(result.accepted[0]?.label).toBe('A | B');
    expect(result.accepted[0]?.url).toBe('https://c.example.test');
  });

  // Reported rather than dropped, so the author can see what went wrong.
  it('reports links with an unsafe scheme instead of accepting them', () => {
    /* eslint-disable no-script-url */
    const result = parseLinks(
      'Bad | javascript:alert(1)\nWorse | data:text/html,x\nGood | https://ok.example.test',
    );
    expect(result.accepted).toHaveLength(1);
    expect(result.accepted[0]?.url).toBe('https://ok.example.test');
    expect(result.rejected).toHaveLength(2);
  });
});

describe('parseChecklist', () => {
  it('splits lines and trims them', () => {
    expect(parseChecklist(' one \n two \n\n three ')).toEqual([
      { text: 'one' },
      { text: 'two' },
      { text: 'three' },
    ]);
  });

  it('returns nothing for empty input', () => {
    expect(parseChecklist('')).toEqual([]);
    expect(parseChecklist('\n\n')).toEqual([]);
  });
});

describe('toParagraphs', () => {
  it('wraps text in a paragraph', () => {
    expect(toParagraphs('Hello')).toBe('<p>Hello</p>');
  });

  it('splits on blank lines and keeps single breaks', () => {
    expect(toParagraphs('One\nstill one\n\nTwo')).toBe('<p>One<br>still one</p><p>Two</p>');
  });

  it('escapes markup so typed angle brackets stay literal', () => {
    expect(toParagraphs('a < b & c > d')).toBe('<p>a &lt; b &amp; c &gt; d</p>');
  });

  it('escapes a script tag rather than storing it', () => {
    const html = toParagraphs('<script>alert(1)</script>');
    expect(html).not.toContain('<script>');
    expect(html).toContain('&lt;script&gt;');
  });

  it('returns empty for blank input', () => {
    expect(toParagraphs('   \n  ')).toBe('');
  });
});
