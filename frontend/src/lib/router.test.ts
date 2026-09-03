import { describe, expect, it } from 'vitest';
import { boardPath, parseRoute, reportPath, routeBoardId } from './router';

describe('parseRoute', () => {
  it('recognises the root', () => {
    expect(parseRoute('/')).toEqual({ name: 'home' });
    expect(parseRoute('')).toEqual({ name: 'home' });
  });

  it('recognises a board', () => {
    expect(parseRoute('/boards/abc123')).toEqual({ name: 'board', boardId: 'abc123' });
  });

  it('tolerates a trailing slash', () => {
    expect(parseRoute('/boards/abc123/')).toEqual({ name: 'board', boardId: 'abc123' });
  });

  it('recognises a board report', () => {
    expect(parseRoute('/boards/abc123/report')).toEqual({ name: 'report', boardId: 'abc123' });
  });

  it('decodes an escaped board id', () => {
    expect(parseRoute('/boards/a%2Fb')).toEqual({ name: 'board', boardId: 'a/b' });
  });

  it('reports anything else as not found', () => {
    expect(parseRoute('/nope')).toEqual({ name: 'notFound', path: '/nope' });
    expect(parseRoute('/boards')).toEqual({ name: 'notFound', path: '/boards' });
    expect(parseRoute('/boards/abc/unknown')).toEqual({
      name: 'notFound',
      path: '/boards/abc/unknown',
    });
  });
});

describe('path builders', () => {
  it('round-trips through parseRoute', () => {
    expect(parseRoute(boardPath('abc123'))).toEqual({ name: 'board', boardId: 'abc123' });
    expect(parseRoute(reportPath('abc123'))).toEqual({ name: 'report', boardId: 'abc123' });
  });

  it('escapes ids', () => {
    expect(boardPath('a/b')).toBe('/boards/a%2Fb');
    expect(parseRoute(boardPath('a/b'))).toEqual({ name: 'board', boardId: 'a/b' });
  });
});

describe('routeBoardId', () => {
  it('extracts the board id where there is one', () => {
    expect(routeBoardId({ name: 'board', boardId: 'x' })).toBe('x');
    expect(routeBoardId({ name: 'report', boardId: 'x' })).toBe('x');
    expect(routeBoardId({ name: 'home' })).toBeNull();
    expect(routeBoardId({ name: 'notFound', path: '/x' })).toBeNull();
  });
});
