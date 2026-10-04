// The only file that knows Heat.js exists.
//
// Heat.js (npm: jheat.js) is a browser-global library. Its dist is an IIFE that
// assigns window.$heat, there is no named or default export, and the shipped
// heat.d.ts is literally `export {}`, so there are no usable types either. Rather
// than let that leak into components, it is imported once here for its side
// effect and exposed through a narrow, typed accessor. Swapping the library out
// later means rewriting this file and ActivityHeatmap.tsx and nothing else.
import 'jheat.js';
import 'jheat.js/dist/heat.js.css';

/**
 * The options we actually pass to `render`.
 *
 * This is deliberately a small subset. The full set lives upstream in
 * src/ts/type.ts (BindingOptions) and has several hundred keys; typing all of
 * them here would be a second source of truth that drifts on every upgrade. Add a
 * key here when a component starts using it.
 */
export interface HeatBindingOptions {
  defaultYear?: number;
  defaultView?: 'map' | 'chart' | 'line' | 'days' | 'months' | 'colorRanges';
  showOnlyDataForYearsAvailable?: boolean;
  sideMenu?: { enabled?: boolean };
  title?: {
    text?: string;
    showYearSelectionDropDown?: boolean;
    showCurrentYearButton?: boolean;
    showRefreshButton?: boolean;
    showExportButton?: boolean;
    showImportButton?: boolean;
    showConfigurationButton?: boolean;
    showClearButton?: boolean;
  };
  guide?: {
    enabled?: boolean;
    colorRangeTogglesEnabled?: boolean;
    showLessAndMoreLabels?: boolean;
    allowTypeAdding?: boolean;
    allowTypeRemoving?: boolean;
  };
  yearlyStatistics?: { enabled?: boolean };
  /** Thresholds for the four colour steps; `cssClassName` names the .day-color-N rule. */
  colorRanges?: {
    id?: string;
    name?: string;
    minimum?: number;
    cssClassName?: string;
    tooltipText?: string;
    visible?: boolean;
  }[];
  views?: {
    map?: {
      enabled?: boolean;
      showMonthNames?: boolean;
      showDayNames?: boolean;
      highlightCurrentDay?: boolean;
    };
    chart?: { enabled?: boolean };
    days?: { enabled?: boolean };
    months?: { enabled?: boolean };
    line?: { enabled?: boolean };
    colorRanges?: { enabled?: boolean };
  };
}

/** The slice of the Heat.js public API (v5.2.0) this app calls. */
export interface HeatApi {
  render(element: HTMLElement, options: HeatBindingOptions): HeatApi;
  /**
   * Types must exist before they can be given data: updateDate silently ignores
   * an unknown type, so add every type first.
   */
  addType(elementId: string, type: string, triggerRefresh?: boolean): HeatApi;
  /** Sets (not adds to) the count for one day. Counts of zero or less are ignored. */
  updateDate(
    elementId: string,
    date: Date,
    count: number,
    type?: string,
    triggerRefresh?: boolean,
  ): HeatApi;
  switchType(elementId: string, type: string): HeatApi;
  refresh(elementId: string): HeatApi;
  /** Restores the element to how it was before `render` and forgets the id. */
  destroy(elementId: string): HeatApi;
  getIds(): string[];
  getYear(elementId: string): number;
  getVersion(): string;
}

declare global {
  interface Window {
    $heat?: HeatApi;
  }
}

/**
 * The Heat.js API, or a clear error if the library did not load.
 *
 * Failing loudly here beats a "cannot read properties of undefined" from deep
 * inside a component; callers that must not take the page down catch it.
 */
export function heat(): HeatApi {
  const api = window.$heat;
  if (!api) {
    throw new Error('Heat.js did not initialise: window.$heat is not defined.');
  }
  return api;
}
