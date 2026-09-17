import { describe, it, expect } from 'vitest';
import { withExcluded, withoutExcluded } from './profileExclusions';

describe('withExcluded', () => {
  it('appends the value', () => {
    expect(withExcluded(['greenhouse'], 'workday')).toEqual(['greenhouse', 'workday']);
  });

  it('adds nothing when the value is already present, whatever its case', () => {
    expect(withExcluded(['Greenhouse'], 'greenhouse')).toEqual(['Greenhouse']);
  });

  it('does not mutate what it was given', () => {
    const before = ['greenhouse'];
    withExcluded(before, 'workday');
    expect(before).toEqual(['greenhouse']);
  });
});

describe('withoutExcluded', () => {
  it('removes only the named value, keeping another one standing', () => {
    expect(withoutExcluded(['greenhouse', 'workday'], 'greenhouse')).toEqual(['workday']);
  });

  it('matches the value whatever its case', () => {
    expect(withoutExcluded(['Greenhouse'], 'greenhouse')).toEqual([]);
  });

  it('is a no-op when the value is not present', () => {
    expect(withoutExcluded(['workday'], 'greenhouse')).toEqual(['workday']);
  });

  it('does not mutate what it was given', () => {
    const before = ['greenhouse', 'workday'];
    withoutExcluded(before, 'greenhouse');
    expect(before).toEqual(['greenhouse', 'workday']);
  });
});
