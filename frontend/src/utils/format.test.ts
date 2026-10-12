import { formatBytes, formatTokens } from './format';
import { estimateTokens } from './tokens';

test('formatTokens shows concrete compact values', () => {
    expect(formatTokens(950)).toBe('950');
    expect(formatTokens(12_345)).toBe('12.3k');
    expect(formatTokens(100_000)).toBe('100k');
    expect(formatTokens(1_500_000)).toBe('1.5M');
});

test('formatBytes', () => {
    expect(formatBytes(78)).toBe('78 B');
    expect(formatBytes(2048)).toBe('2 KB');
});

test('estimateTokens matches the backend estimate', () => {
    // Same cases as internal/knowledge TestEstimateTokens.
    expect(estimateTokens('')).toBe(0);
    expect(estimateTokens('abcd')).toBe(1);
    expect(estimateTokens('abcde')).toBe(2);
    expect(estimateTokens('你好世界')).toBe(4);
});
