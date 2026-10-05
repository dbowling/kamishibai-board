import { useMemo } from 'react';
import { timeZoneOptions } from './format';

interface TimeZoneSelectProps {
  label: string;
  /** The selected IANA zone, or '' for the empty option. */
  value: string;
  onChange: (zone: string) => void;
  /** What the empty option reads, e.g. "Instance default (America/New_York)". */
  emptyLabel: string;
  hint?: string;
}

/** A labelled select of IANA zones whose first option means "none chosen". */
export function TimeZoneSelect({ label, value, onChange, emptyLabel, hint }: TimeZoneSelectProps) {
  const zones = useMemo(() => timeZoneOptions(value), [value]);

  return (
    <label className="field">
      <span className="field__label">{label}</span>
      <select
        className="field__input"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        <option value="">{emptyLabel}</option>
        {zones.map((zone) => (
          <option key={zone} value={zone}>
            {zone}
          </option>
        ))}
      </select>
      {hint && <span className="field__hint">{hint}</span>}
    </label>
  );
}
