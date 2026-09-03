import { describe, expect, it } from 'vitest';
import { displayHost, safeUrl, sanitizeInstructions } from './sanitize';

// Card instructions are written by team members and rendered in everybody else's
// browser, so these tests are about containment rather than formatting.

describe('sanitizeInstructions', () => {
  it('keeps the markup a runbook actually needs', () => {
    const html =
      '<p>Check the <strong>backup</strong> dashboard.</p><ul><li>Step one</li></ul>' +
      '<a href="https://grafana.example.test/d/backups">Dashboard</a>';
    const clean = sanitizeInstructions(html);

    expect(clean).toContain('<p>');
    expect(clean).toContain('<strong>');
    expect(clean).toContain('<li>Step one</li>');
    expect(clean).toContain('href="https://grafana.example.test/d/backups"');
  });

  it('strips scripts and does not leave their source behind as text', () => {
    const clean = sanitizeInstructions('<p>Hi</p><script>alert("xss")</script>');
    expect(clean).not.toContain('script');
    expect(clean).not.toContain('alert');
    expect(clean).toContain('Hi');
  });

  it('removes inline event handlers', () => {
    const clean = sanitizeInstructions('<p onclick="steal()">Click me</p>');
    expect(clean).not.toContain('onclick');
    expect(clean).toContain('Click me');
  });

  it('removes javascript: hrefs but keeps the link text', () => {
    /* eslint-disable no-script-url */
    const clean = sanitizeInstructions('<a href="javascript:alert(1)">Press</a>');
    expect(clean).not.toContain('javascript:');
    expect(clean).toContain('Press');
  });

  it('drops embedded frames and objects', () => {
    for (const html of [
      '<iframe src="https://evil.example"></iframe>',
      '<object data="x"></object>',
      '<embed src="x">',
      '<form action="/x"><input name="a"></form>',
    ]) {
      const clean = sanitizeInstructions(html);
      expect(clean).not.toMatch(/iframe|object|embed|<form|<input/i);
    }
  });

  it('removes style attributes and img tags', () => {
    const clean = sanitizeInstructions(
      '<p style="position:fixed;top:0">x</p><img src=x onerror=alert(1)>',
    );
    expect(clean).not.toContain('style=');
    expect(clean).not.toContain('<img');
    expect(clean).not.toContain('onerror');
  });

  it('handles empty input', () => {
    expect(sanitizeInstructions('')).toBe('');
  });
});

describe('safeUrl', () => {
  it('accepts http, https and mailto', () => {
    expect(safeUrl('https://grafana.example.test/d/backups')).toBe(
      'https://grafana.example.test/d/backups',
    );
    expect(safeUrl('http://wiki.example.test/runbook')).toBe('http://wiki.example.test/runbook');
    expect(safeUrl('mailto:oncall@example.test')).toBe('mailto:oncall@example.test');
  });

  it('accepts relative and anchor links unchanged', () => {
    expect(safeUrl('/boards/abc')).toBe('/boards/abc');
    expect(safeUrl('#section')).toBe('#section');
  });

  it('rejects dangerous schemes', () => {
    /* eslint-disable no-script-url */
    for (const url of [
      'javascript:alert(1)',
      'JavaScript:alert(1)',
      '  javascript:alert(1)  ',
      'data:text/html;base64,PHNjcmlwdD4=',
      'vbscript:msgbox(1)',
      'file:///etc/passwd',
    ]) {
      expect(safeUrl(url), url).toBeNull();
    }
  });

  it('rejects empty and malformed values', () => {
    expect(safeUrl('')).toBeNull();
    expect(safeUrl('   ')).toBeNull();
    expect(safeUrl('not a url')).toBeNull();
  });
});

describe('displayHost', () => {
  it('extracts the host', () => {
    expect(displayHost('https://grafana.example.test/d/backups')).toBe('grafana.example.test');
  });

  it('returns empty for a non-absolute URL', () => {
    expect(displayHost('/boards/abc')).toBe('');
  });
});
