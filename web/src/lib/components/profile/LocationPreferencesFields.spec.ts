import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import LocationPreferencesFields from './LocationPreferencesFields.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'ru' }, url: new URL('http://localhost/') },
}));

describe('LocationPreferencesFields', () => {
  it('renders its own strings in the resolved locale, leaving facet labels untranslated', () => {
    render(LocationPreferencesFields, {
      props: { value: null, derivedLocation: null, onChange: vi.fn() },
    });

    // The component's own prose renders in Russian.
    expect(screen.getByText('Только для настройки фильтров вакансий — необязательно.')).toBeTruthy();
    expect(screen.getByText('Формат работы')).toBeTruthy();
    expect(screen.getByText('Где вы базируетесь')).toBeTruthy();
    expect(screen.getByPlaceholderText('Город или страна')).toBeTruthy();
    expect(
      screen.getByText('Выберите формат работы выше, чтобы указать, где вы можете работать.'),
    ).toBeTruthy();

    // Facet dictionary values (WORK_MODE_OPTIONS) are untouched by this catalog — they
    // stay in whatever the dictionary itself is written in.
    expect(screen.getByText('Remote')).toBeTruthy();
  });

  it('renders the relocation section strings in Russian once relocation is open', () => {
    // wantsPhysical (onsite/hybrid) gates the relocation block, and relocOpen seeds
    // it expanded — the only way to reach "Open to relocation", "Where you'd
    // relocate", and the "Add a city" picker without simulating a click first.
    render(LocationPreferencesFields, {
      props: {
        value: {
          work_modes: ['onsite'],
          remote: {},
          base: {},
          relocation: { open: true },
        },
        derivedLocation: null,
        onChange: vi.fn(),
      },
    });

    expect(screen.getByText('Готов(а) к переезду')).toBeTruthy();
    expect(screen.getByText('Куда вы готовы переехать (пусто = куда угодно)')).toBeTruthy();
    expect(screen.getByPlaceholderText('Добавить город')).toBeTruthy();
  });
});
